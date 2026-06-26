package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pashagolub/pgxmock/v4"

	"github.com/pizdagladki/full/services/reports/internal/api/domain"
	"github.com/pizdagladki/full/services/reports/internal/api/repository"
)

func TestInsertBugReport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		report  domain.BugReport
		setup   func(m pgxmock.PgxPoolIface)
		wantErr bool
	}{
		{
			// criterion: 1 — mobile report with empty object_key writes NULL via NULLIF
			name: "mobile report with empty object_key inserts successfully",
			report: domain.BugReport{
				UserID:      42,
				Device:      "mobile",
				Description: "app crash",
				ObjectKey:   "",
			},
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectExec(`INSERT INTO bug_reports`).
					WithArgs(int64(42), "mobile", "app crash", "").
					WillReturnResult(pgxmock.NewResult("INSERT", 1))
			},
		},
		{
			// criterion: 2 — pc report with object_key writes the key
			name: "pc report with object_key inserts successfully",
			report: domain.BugReport{
				UserID:      7,
				Device:      "pc",
				Description: "freeze",
				ObjectKey:   "7-1234567890.webm",
			},
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectExec(`INSERT INTO bug_reports`).
					WithArgs(int64(7), "pc", "freeze", "7-1234567890.webm").
					WillReturnResult(pgxmock.NewResult("INSERT", 1))
			},
		},
		{
			// criterion: 1 (edge) — DB error is wrapped and propagated
			name: "db error is propagated",
			report: domain.BugReport{
				UserID: 1,
				Device: "mobile",
			},
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectExec(`INSERT INTO bug_reports`).
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

			repo := repository.NewBugReportsRepository(mock)
			err = repo.InsertBugReport(context.Background(), tt.report)

			if tt.wantErr {
				if err == nil {
					t.Fatal("InsertBugReport() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("InsertBugReport() unexpected error = %v", err)
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}
