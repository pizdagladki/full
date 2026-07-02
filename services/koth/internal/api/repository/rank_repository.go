package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/pizdagladki/full/services/koth/internal/api/domain"
)

// rankPoolIface is the minimal pool interface required by rankRepository.
// Both *pgxpool.Pool and pgxmock.PgxPoolIface satisfy this interface, allowing
// tests to inject a mock without a live database.
type rankPoolIface interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

type rankRepository struct {
	pool rankPoolIface
}

// NewRankRepository returns a RankRepository backed by pool.
// In production pass *pgxpool.Pool; in tests pass pgxmock.PgxPoolIface.
func NewRankRepository(pool rankPoolIface) RankRepository {
	return &rankRepository{pool: pool}
}

const getRankSQL = `
SELECT user_id, day, current_rank, best_hold_ms
FROM hill_ranks
WHERE user_id = $1 AND day = $2::date`

func (r *rankRepository) GetRank(ctx context.Context, userID int64, day time.Time) (*domain.HillRank, error) {
	var hr domain.HillRank

	err := r.pool.QueryRow(ctx, getRankSQL, userID, day).
		Scan(&hr.UserID, &hr.Day, &hr.Rank, &hr.BestHoldMs)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRankNotFound
		}

		return nil, fmt.Errorf("get rank: %w", err)
	}

	return &hr, nil
}

const upsertRankSQL = `
INSERT INTO hill_ranks (user_id, day, current_rank, best_hold_ms)
VALUES ($1, $2::date, $3, $4)
ON CONFLICT (user_id, day) DO UPDATE
SET current_rank = EXCLUDED.current_rank,
    best_hold_ms  = EXCLUDED.best_hold_ms,
    updated_at    = now()
WHERE EXCLUDED.current_rank > hill_ranks.current_rank`

func (r *rankRepository) UpsertRank(ctx context.Context, userID int64, day time.Time, newRank, bestHoldMs int) error {
	_, err := r.pool.Exec(ctx, upsertRankSQL, userID, day, newRank, bestHoldMs)
	if err != nil {
		return fmt.Errorf("upsert rank: %w", err)
	}

	return nil
}

const rankDistributionSQL = `
SELECT current_rank, COUNT(*)
FROM hill_ranks
WHERE day = $1::date
GROUP BY current_rank
ORDER BY current_rank`

func (r *rankRepository) RankDistribution(ctx context.Context, day time.Time) ([]domain.RankCount, error) {
	rows, err := r.pool.Query(ctx, rankDistributionSQL, day)
	if err != nil {
		return nil, fmt.Errorf("rank distribution: %w", err)
	}
	defer rows.Close()

	counts := make([]domain.RankCount, 0)

	for rows.Next() {
		var rc domain.RankCount

		err = rows.Scan(&rc.Rank, &rc.Count)
		if err != nil {
			return nil, fmt.Errorf("scan rank count: %w", err)
		}

		counts = append(counts, rc)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate rank counts: %w", err)
	}

	return counts, nil
}
