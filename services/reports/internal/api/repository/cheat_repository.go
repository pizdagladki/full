package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/pizdagladki/full/services/reports/internal/api/domain"
)

// PgxPool is the minimal pool interface required by cheatReportsRepository.
// Both *pgxpool.Pool and pgxmock.PgxPoolIface satisfy this interface, which
// allows tests to inject a mock without a live database.
type PgxPool interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type cheatReportsRepository struct {
	pool PgxPool
}

// NewCheatReportsRepository constructs a Postgres-backed CheatReportsRepository.
// In production pass *pgxpool.Pool; in tests pass pgxmock.PgxPoolIface.
func NewCheatReportsRepository(pool PgxPool) CheatReportsRepository {
	return &cheatReportsRepository{pool: pool}
}

// UpsertCheatReport inserts a cheat report. ON CONFLICT (reporter_id, match_id)
// DO NOTHING makes the operation idempotent.
func (r *cheatReportsRepository) UpsertCheatReport(ctx context.Context, report domain.CheatReport) error {
	const q = `
INSERT INTO cheat_reports (reporter_id, reported_id, match_id)
VALUES ($1, $2, $3)
ON CONFLICT (reporter_id, match_id) DO NOTHING`

	_, err := r.pool.Exec(ctx, q, report.ReporterID, report.ReportedID, report.MatchID)
	if err != nil {
		return fmt.Errorf("upsert cheat report: %w", err)
	}

	return nil
}

// CountRecentCheatReports counts cheat reports for reported_id across their
// lastNMatches most recent distinct match IDs.
func (r *cheatReportsRepository) CountRecentCheatReports(
	ctx context.Context,
	reportedID int64,
	lastNMatches int,
) (int, error) {
	const q = `
SELECT COUNT(*)
FROM cheat_reports
WHERE reported_id = $1
  AND match_id IN (
    SELECT match_id
    FROM (
      SELECT match_id, MAX(created_at) AS last_seen
      FROM cheat_reports
      WHERE reported_id = $1
      GROUP BY match_id
      ORDER BY last_seen DESC
      LIMIT $2
    ) recent
  )`

	var count int

	err := r.pool.QueryRow(ctx, q, reportedID, lastNMatches).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count recent cheat reports: %w", err)
	}

	return count, nil
}
