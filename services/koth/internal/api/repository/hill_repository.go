package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pizdagladki/full/services/koth/internal/api/domain"
)

// hillQueryRow is the minimal read contract shared by a bare pool and a pgx.Tx,
// letting scanCurrentKing run against either.
type hillQueryRow interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// hillDB is the minimal pgx interface required by hillRepository. Both
// *pgxpool.Pool and pgxmock.PgxPoolIface satisfy it, allowing tests to inject
// a mock without a live database.
type hillDB interface {
	hillQueryRow
	Begin(ctx context.Context) (pgx.Tx, error)
}

type hillRepository struct {
	db hillDB
}

// NewHillRepository returns a HillRepository backed by db.
// In production pass *pgxpool.Pool; in tests pass pgxmock.PgxPoolIface.
func NewHillRepository(db hillDB) HillRepository {
	return &hillRepository{db: db}
}

const currentKingSQL = `
SELECT id, user_id, clip_id, blink_ts_ms, started_at
FROM king_reigns
WHERE hill_type = $1 AND ended_at IS NULL`

// scanCurrentKing runs currentKingSQL against q (a pool or an open tx) and
// maps the row to a domain.KingReign. Returns ErrHillNotFound when no current
// reign exists for hillType.
func scanCurrentKing(ctx context.Context, q hillQueryRow, hillType domain.HillType) (*domain.KingReign, error) {
	var (
		king   domain.KingReign
		clipID sql.NullString
	)

	err := q.QueryRow(ctx, currentKingSQL, string(hillType)).
		Scan(&king.ID, &king.UserID, &clipID, &king.BlinkTsMs, &king.StartedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrHillNotFound
		}

		return nil, fmt.Errorf("current king: %w", err)
	}

	king.HillType = hillType
	king.ClipID = clipID.String

	return &king, nil
}

func (r *hillRepository) CurrentKing(ctx context.Context, hillType domain.HillType) (*domain.KingReign, error) {
	return scanCurrentKing(ctx, r.db, hillType)
}

const (
	// lockHillSQL serializes all challenges against a given hill_type: the
	// advisory lock is held for the lifetime of the transaction, so a second
	// concurrent challenge blocks here until the first commits (or rolls
	// back), then re-reads the current king fresh below rather than racing.
	lockHillSQL = `SELECT pg_advisory_xact_lock(hashtext($1))`

	closeReignSQL = `
UPDATE king_reigns
SET ended_at = now()
WHERE id = $1`

	openReignSQL = `
INSERT INTO king_reigns (hill_type, user_id, clip_id, blink_ts_ms)
VALUES ($1, $2, $3, $4)
RETURNING id, started_at`
)

func (r *hillRepository) Challenge(
	ctx context.Context, hillType domain.HillType, userID int64, survivedMs int, newClipID string,
) (domain.ChallengeOutcome, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.ChallengeOutcome{}, fmt.Errorf("begin tx: %w", err)
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	_, err = tx.Exec(ctx, lockHillSQL, string(hillType))
	if err != nil {
		return domain.ChallengeOutcome{}, fmt.Errorf("lock hill: %w", err)
	}

	king, err := scanCurrentKing(ctx, tx, hillType)
	if err != nil {
		return domain.ChallengeOutcome{}, err
	}

	if survivedMs < king.BlinkTsMs {
		err = tx.Commit(ctx)
		if err != nil {
			return domain.ChallengeOutcome{}, fmt.Errorf("commit tx: %w", err)
		}

		return domain.ChallengeOutcome{Won: false, King: *king}, nil
	}

	_, err = tx.Exec(ctx, closeReignSQL, king.ID)
	if err != nil {
		return domain.ChallengeOutcome{}, fmt.Errorf("close reign: %w", err)
	}

	newKing := domain.KingReign{
		HillType:  hillType,
		UserID:    userID,
		ClipID:    newClipID,
		BlinkTsMs: survivedMs,
	}

	err = tx.QueryRow(ctx, openReignSQL, string(hillType), userID, newClipID, survivedMs).
		Scan(&newKing.ID, &newKing.StartedAt)
	if err != nil {
		return domain.ChallengeOutcome{}, fmt.Errorf("open reign: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return domain.ChallengeOutcome{}, fmt.Errorf("commit tx: %w", err)
	}

	return domain.ChallengeOutcome{Won: true, King: newKing}, nil
}

// CloseIfStale closes the current reign for hillType when it started before
// periodStart, returning the pre-close snapshot for reward/expiry handling.
// It takes the same advisory lock as Challenge, serializing a reset-close
// against a concurrent challenge on the same hill.
func (r *hillRepository) CloseIfStale(
	ctx context.Context, hillType domain.HillType, periodStart time.Time,
) (*domain.KingReign, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	_, err = tx.Exec(ctx, lockHillSQL, string(hillType))
	if err != nil {
		return nil, fmt.Errorf("lock hill: %w", err)
	}

	king, scanErr := scanCurrentKing(ctx, tx, hillType)
	if scanErr != nil {
		if errors.Is(scanErr, ErrHillNotFound) {
			err = tx.Commit(ctx)
			if err != nil {
				return nil, fmt.Errorf("commit tx: %w", err)
			}

			return nil, nil //nolint:nilnil // no current reign: nothing to close, not an error
		}

		err = scanErr

		return nil, err
	}

	if !king.StartedAt.Before(periodStart) {
		// The current reign already started within this period — either it
		// was already reset, or it was (re)seeded after the boundary rolled
		// over. Nothing to do; commit the read-only tx and report a no-op.
		err = tx.Commit(ctx)
		if err != nil {
			return nil, fmt.Errorf("commit tx: %w", err)
		}

		return nil, nil //nolint:nilnil // fresh reign: nothing to close, not an error
	}

	_, err = tx.Exec(ctx, closeReignSQL, king.ID)
	if err != nil {
		return nil, fmt.Errorf("close reign: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return king, nil
}
