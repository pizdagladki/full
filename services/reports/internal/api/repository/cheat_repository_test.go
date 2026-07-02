package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pashagolub/pgxmock/v4"

	"github.com/pizdagladki/full/services/reports/internal/api/domain"
	"github.com/pizdagladki/full/services/reports/internal/api/repository"
)

func TestUpsertCheatReport(t *testing.T) {
	t.Parallel()

	report := domain.CheatReport{ReporterID: 1, ReportedID: 2, MatchID: "match-xyz"}

	tests := []struct {
		name    string
		setup   func(m pgxmock.PgxPoolIface)
		wantErr bool
	}{
		{
			// criterion: 1 — new report inserted successfully
			name: "insert new report succeeds",
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectExec(`INSERT INTO cheat_reports`).
					WithArgs(report.ReporterID, report.ReportedID, report.MatchID).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))
			},
		},
		{
			// criterion: 1 — idempotent: ON CONFLICT DO NOTHING returns 0 rows affected, still no error
			name: "idempotent conflict returns no error",
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectExec(`INSERT INTO cheat_reports`).
					WithArgs(report.ReporterID, report.ReportedID, report.MatchID).
					WillReturnResult(pgxmock.NewResult("INSERT", 0)) // 0 rows = conflict ignored
			},
		},
		{
			// criterion: 1 — DB error is wrapped and propagated
			name: "exec error is propagated",
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectExec(`INSERT INTO cheat_reports`).
					WillReturnError(errors.New("connection refused"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock, err := pgxmock.NewPool()
			if err != nil {
				t.Fatalf("pgxmock.NewPool() error = %v", err)
			}
			defer mock.Close()

			tt.setup(mock)

			repo := repository.NewCheatReportsRepository(mock)
			err = repo.UpsertCheatReport(context.Background(), report)

			if tt.wantErr {
				if err == nil {
					t.Fatal("UpsertCheatReport() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("UpsertCheatReport() unexpected error = %v", err)
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestCountRecentCheatReports(t *testing.T) {
	t.Parallel()

	const reportedID = int64(2)
	const lastN = 10

	// The regex pins the key structural elements: GROUP BY match_id ensures
	// per-match dedup (each match counted once), LIMIT ensures the
	// 10-recent-matches window.
	const distinctLimitRegex = `(?i)SELECT\s+COUNT.*GROUP\s+BY.*match_id.*LIMIT`

	tests := []struct {
		name      string
		setup     func(m pgxmock.PgxPoolIface)
		wantCount int
		wantErr   bool
	}{
		{
			// criterion: 2 — count returned correctly (below threshold)
			name: "returns count below threshold",
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"count"}).AddRow(4)
				// The regex pins the key structural elements: GROUP BY match_id ensures
				// per-match dedup, LIMIT ensures the 10-recent-matches window.
				m.ExpectQuery(distinctLimitRegex).
					WithArgs(reportedID, lastN).
					WillReturnRows(rows)
			},
			wantCount: 4,
		},
		{
			// criterion: 2 — count returned correctly (at threshold)
			name: "returns count at threshold",
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"count"}).AddRow(5)
				// The regex pins the key structural elements: GROUP BY match_id ensures
				// per-match dedup, LIMIT ensures the 10-recent-matches window.
				m.ExpectQuery(distinctLimitRegex).
					WithArgs(reportedID, lastN).
					WillReturnRows(rows)
			},
			wantCount: 5,
		},
		{
			// criterion: 2 — DB error is wrapped and propagated
			name: "query error is propagated",
			setup: func(m pgxmock.PgxPoolIface) {
				// The regex pins the key structural elements: GROUP BY match_id ensures
				// per-match dedup, LIMIT ensures the 10-recent-matches window.
				m.ExpectQuery(distinctLimitRegex).
					WillReturnError(errors.New("db error"))
			},
			wantErr: true,
		},
		{
			// criterion: 2 — uses GROUP BY match_id and LIMIT in query (structural regression guard)
			// This case uses the specific regex expectation: a query that drops GROUP BY or
			// LIMIT would fail the mock expectation with "no expectations matched".
			name: "uses GROUP BY match_id and LIMIT in query",
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"count"}).AddRow(3)
				// The regex pins the key structural elements: GROUP BY match_id ensures
				// per-match dedup, LIMIT ensures the 10-recent-matches window.
				m.ExpectQuery(distinctLimitRegex).
					WithArgs(reportedID, lastN).
					WillReturnRows(rows)
			},
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock, err := pgxmock.NewPool()
			if err != nil {
				t.Fatalf("pgxmock.NewPool() error = %v", err)
			}
			defer mock.Close()

			tt.setup(mock)

			repo := repository.NewCheatReportsRepository(mock)
			count, err := repo.CountRecentCheatReports(context.Background(), reportedID, lastN)

			if tt.wantErr {
				if err == nil {
					t.Fatal("CountRecentCheatReports() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("CountRecentCheatReports() unexpected error = %v", err)
			}

			if count != tt.wantCount {
				t.Errorf("count = %d, want %d", count, tt.wantCount)
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}
