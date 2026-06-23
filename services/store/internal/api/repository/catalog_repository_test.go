package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pashagolub/pgxmock/v4"

	"github.com/pizdagladki/full/services/store/internal/api/repository"
)

func ptr[T any](v T) *T { return &v }

func TestCatalogRepository_ListProducts(t *testing.T) {
	t.Parallel()

	cols := []string{"id", "kind", "tier", "name", "price_cents", "is_free"}
	tier1 := 1

	tests := []struct {
		name    string
		kind    *string
		setup   func(m pgxmock.PgxPoolIface)
		wantLen int
		wantErr bool
	}{
		{
			name: "all products returned when kind is nil",
			kind: nil,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(cols).
					AddRow(int64(1), "distraction", &tier1, "Spinner", 0, true).
					AddRow(int64(2), "edit", nil, "Blur", 100, false)
				m.ExpectQuery(`SELECT`).WillReturnRows(rows)
			},
			wantLen: 2,
		},
		{
			name: "filter by kind=distraction returns only matching rows",
			kind: ptr("distraction"),
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(cols).
					AddRow(int64(1), "distraction", &tier1, "Spinner", 0, true)
				m.ExpectQuery(`SELECT`).WithArgs("distraction").WillReturnRows(rows)
			},
			wantLen: 1,
		},
		{
			name: "empty result returns empty slice not nil",
			kind: nil,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(cols)
				m.ExpectQuery(`SELECT`).WillReturnRows(rows)
			},
			wantLen: 0,
		},
		{
			name: "query error is propagated",
			kind: nil,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`SELECT`).WillReturnError(errors.New("connection refused"))
			},
			wantErr: true,
		},
		{
			name: "scan error is propagated",
			kind: nil,
			setup: func(m pgxmock.PgxPoolIface) {
				// Return a row with wrong column count to trigger a scan error.
				rows := pgxmock.NewRows([]string{"id"}).AddRow(int64(1))
				m.ExpectQuery(`SELECT`).WillReturnRows(rows)
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

			repo := repository.NewCatalogRepository(mock)

			got, err := repo.ListProducts(context.Background(), tt.kind)

			if tt.wantErr {
				if err == nil {
					t.Fatal("ListProducts() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("ListProducts() unexpected error = %v", err)
			}

			if len(got) != tt.wantLen {
				t.Errorf("len(products) = %d, want %d", len(got), tt.wantLen)
			}

			// Verify non-nil slice even for empty result.
			if got == nil {
				t.Error("ListProducts() returned nil, want non-nil slice")
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled pgxmock expectations: %v", err)
			}
		})
	}
}
