package repository

import (
	"context"
	"fmt"

	"github.com/pizdagladki/full/services/reports/internal/api/domain"
)

type bugReportsRepository struct {
	pool PgxPool
}

// NewBugReportsRepository constructs a Postgres-backed BugReportsRepository.
func NewBugReportsRepository(pool PgxPool) BugReportsRepository {
	return &bugReportsRepository{pool: pool}
}

// InsertBugReport inserts a bug report row. A NULL object_key is stored when
// ObjectKey is an empty string (NULLIF($4, ”)).
func (r *bugReportsRepository) InsertBugReport(ctx context.Context, report domain.BugReport) error {
	const q = `
INSERT INTO bug_reports (user_id, device, description, object_key)
VALUES ($1, $2, $3, NULLIF($4, ''))`

	_, err := r.pool.Exec(ctx, q, report.UserID, report.Device, report.Description, report.ObjectKey)
	if err != nil {
		return fmt.Errorf("insert bug report: %w", err)
	}

	return nil
}
