package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pizdagladki/full/services/ratings/internal/api/domain"
)

// DB is the minimal pgx interface needed by ratingsRepository.
// *pgxpool.Pool satisfies it; pgxmock's mock pool satisfies it in tests.
type DB interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type ratingsRepository struct {
	db DB
}

// NewRatingsRepository builds a RatingsRepository backed by pool.
func NewRatingsRepository(pool *pgxpool.Pool) RatingsRepository {
	return NewRatingsRepositoryFromDB(pool)
}

// NewRatingsRepositoryFromDB builds a RatingsRepository backed by any DB.
// This exported form allows tests and alternative wiring (e.g. pgxmock) to inject
// a custom DB implementation without depending on a real *pgxpool.Pool.
func NewRatingsRepositoryFromDB(db DB) RatingsRepository {
	return &ratingsRepository{db: db}
}

// ─── ApplyMatchResult ─────────────────────────────────────────────────────────

const (
	sqlUpsertDefault = `
		INSERT INTO ratings (user_id, elo, level, games_played, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (user_id) DO NOTHING`

	sqlLockRating = `
		SELECT elo, level, games_played
		FROM   ratings
		WHERE  user_id = $1
		FOR UPDATE`

	sqlUpdateRating = `
		UPDATE ratings
		SET    elo          = $2,
		       level        = $3,
		       games_played = $4,
		       updated_at   = now()
		WHERE  user_id = $1`

	sqlInsertMatchResult = `
		INSERT INTO match_results
		       (winner_id, loser_id, mode, winner_elo_delta, loser_elo_delta, duration_ms)
		VALUES ($1,        $2,       $3,   $4,               $5,              $6)`
)

func (r *ratingsRepository) ApplyMatchResult(ctx context.Context, input domain.MatchInput) (domain.MatchResult, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.MatchResult{}, fmt.Errorf("begin tx: %w", err)
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	// Determine canonical (ascending) lock order to prevent AB/BA deadlocks when
	// two concurrent matches involve the same pair with winner/loser swapped.
	firstID, secondID := input.WinnerID, input.LoserID
	if firstID > secondID {
		firstID, secondID = secondID, firstID
	}

	// (a) Materialize default rows for both players if absent — in canonical order.
	err = upsertDefault(ctx, tx, firstID)
	if err != nil {
		return domain.MatchResult{}, fmt.Errorf("upsert default for %d: %w", firstID, err)
	}

	err = upsertDefault(ctx, tx, secondID)
	if err != nil {
		return domain.MatchResult{}, fmt.Errorf("upsert default for %d: %w", secondID, err)
	}

	// (b) SELECT … FOR UPDATE both rows in canonical order.
	var firstRating, secondRating domain.Rating
	firstRating.UserID = firstID
	secondRating.UserID = secondID

	err = tx.QueryRow(ctx, sqlLockRating, firstID).
		Scan(&firstRating.ELO, &firstRating.Level, &firstRating.GamesPlayed)
	if err != nil {
		return domain.MatchResult{}, fmt.Errorf("lock user %d: %w", firstID, err)
	}

	err = tx.QueryRow(ctx, sqlLockRating, secondID).
		Scan(&secondRating.ELO, &secondRating.Level, &secondRating.GamesPlayed)
	if err != nil {
		return domain.MatchResult{}, fmt.Errorf("lock user %d: %w", secondID, err)
	}

	// Map back to winner/loser regardless of lock order.
	var winnerBefore, loserBefore domain.Rating
	if input.WinnerID == firstID {
		winnerBefore, loserBefore = firstRating, secondRating
	} else {
		winnerBefore, loserBefore = secondRating, firstRating
	}

	// (c) Compute deltas via the pure domain functions.
	winnerDelta, loserDelta := domain.CalcELODeltas(
		winnerBefore.ELO, loserBefore.ELO,
		winnerBefore.GamesPlayed, loserBefore.GamesPlayed,
	)

	// (d) Compute new values.
	newWinnerELO := winnerBefore.ELO + winnerDelta
	newLoserELO := loserBefore.ELO + loserDelta

	newWinner := domain.Rating{
		UserID:      input.WinnerID,
		ELO:         newWinnerELO,
		Level:       domain.LevelForELO(newWinnerELO),
		GamesPlayed: winnerBefore.GamesPlayed + 1,
	}
	newLoser := domain.Rating{
		UserID:      input.LoserID,
		ELO:         newLoserELO,
		Level:       domain.LevelForELO(newLoserELO),
		GamesPlayed: loserBefore.GamesPlayed + 1,
	}

	// UPDATE both rows.
	err = execUpdate(ctx, tx, newWinner)
	if err != nil {
		return domain.MatchResult{}, fmt.Errorf("update winner %d: %w", input.WinnerID, err)
	}

	err = execUpdate(ctx, tx, newLoser)
	if err != nil {
		return domain.MatchResult{}, fmt.Errorf("update loser %d: %w", input.LoserID, err)
	}

	// (e) INSERT match_results row.
	_, err = tx.Exec(ctx, sqlInsertMatchResult,
		input.WinnerID, input.LoserID, input.Mode,
		winnerDelta, loserDelta, input.DurationMS)
	if err != nil {
		return domain.MatchResult{}, fmt.Errorf("insert match result: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return domain.MatchResult{}, fmt.Errorf("commit tx: %w", err)
	}

	return domain.MatchResult{
		Winner:      newWinner,
		Loser:       newLoser,
		WinnerDelta: winnerDelta,
		LoserDelta:  loserDelta,
	}, nil
}

func upsertDefault(ctx context.Context, tx pgx.Tx, userID int64) error {
	_, err := tx.Exec(ctx, sqlUpsertDefault,
		userID, domain.DefaultELO, domain.DefaultLevel, domain.DefaultGamesPlayed)

	return err
}

func execUpdate(ctx context.Context, tx pgx.Tx, r domain.Rating) error {
	_, err := tx.Exec(ctx, sqlUpdateRating, r.UserID, r.ELO, r.Level, r.GamesPlayed)

	return err
}

// ─── GetRating ────────────────────────────────────────────────────────────────

const sqlGetRating = `
	SELECT elo, level, games_played
	FROM   ratings
	WHERE  user_id = $1`

func (r *ratingsRepository) GetRating(ctx context.Context, userID int64) (domain.Rating, error) {
	rating := domain.Rating{
		UserID:      userID,
		ELO:         domain.DefaultELO,
		Level:       domain.DefaultLevel,
		GamesPlayed: domain.DefaultGamesPlayed,
	}

	err := r.db.QueryRow(ctx, sqlGetRating, userID).
		Scan(&rating.ELO, &rating.Level, &rating.GamesPlayed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return rating, nil
		}

		return domain.Rating{}, fmt.Errorf("get rating for %d: %w", userID, err)
	}

	return rating, nil
}
