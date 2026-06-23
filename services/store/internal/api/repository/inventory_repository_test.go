package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pashagolub/pgxmock/v4"

	"github.com/pizdagladki/full/services/store/internal/api/repository"
)

func TestInventoryRepository_ListByUser(t *testing.T) {
	t.Parallel()

	cols := []string{"product_id", "quantity"}

	tests := []struct {
		name    string
		userID  int64
		setup   func(m pgxmock.PgxPoolIface)
		wantLen int
		wantErr bool
	}{
		{
			name:   "owned items returned",
			userID: 42,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(cols).
					AddRow(int64(1), 3).
					AddRow(int64(5), 1)
				m.ExpectQuery(`SELECT`).WithArgs(int64(42)).WillReturnRows(rows)
			},
			wantLen: 2,
		},
		{
			name:   "empty inventory returns empty slice not nil",
			userID: 99,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(cols)
				m.ExpectQuery(`SELECT`).WithArgs(int64(99)).WillReturnRows(rows)
			},
			wantLen: 0,
		},
		{
			name:   "query error is propagated",
			userID: 1,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`SELECT`).WithArgs(int64(1)).
					WillReturnError(errors.New("timeout"))
			},
			wantErr: true,
		},
		{
			name:   "scan error is propagated",
			userID: 2,
			setup: func(m pgxmock.PgxPoolIface) {
				// Wrong number of columns triggers a scan error.
				rows := pgxmock.NewRows([]string{"product_id"}).AddRow(int64(1))
				m.ExpectQuery(`SELECT`).WithArgs(int64(2)).WillReturnRows(rows)
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

			repo := repository.NewInventoryRepository(mock)

			got, err := repo.ListByUser(context.Background(), tt.userID)

			if tt.wantErr {
				if err == nil {
					t.Fatal("ListByUser() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("ListByUser() unexpected error = %v", err)
			}

			if len(got) != tt.wantLen {
				t.Errorf("len(items) = %d, want %d", len(got), tt.wantLen)
			}

			if got == nil {
				t.Error("ListByUser() returned nil, want non-nil slice")
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled pgxmock expectations: %v", err)
			}
		})
	}
}
