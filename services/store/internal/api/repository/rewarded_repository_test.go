package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/pizdagladki/full/services/store/internal/api/domain"
	"github.com/pizdagladki/full/services/store/internal/api/repository"
)

func TestRewardedRepository_GetProduct(t *testing.T) {
	t.Parallel()

	cols := []string{"id", "kind", "tier", "name", "price_cents", "is_free", "points_price"}

	tests := []struct {
		name       string
		productID  int64
		setup      func(m pgxmock.PgxPoolIface)
		wantID     int64
		wantKind   string
		wantIsFree bool
		wantErr    bool
		wantErrIs  error
	}{
		{
			// criterion: 1 — GetProduct returns the product (incl. is_free) when found.
			name:      "found returns product",
			productID: 42,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(cols).
					AddRow(int64(42), domain.KindDistraction, nil, "Spinner", 0, true, nil)
				m.ExpectQuery(`SELECT`).WithArgs(int64(42)).WillReturnRows(rows)
			},
			wantID:     42,
			wantKind:   domain.KindDistraction,
			wantIsFree: true,
		},
		{
			// criterion: 4 — GetProduct returns ErrProductNotFound for pgx.ErrNoRows,
			// which the handler maps to 404.
			name:      "not found returns ErrProductNotFound",
			productID: 99,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`SELECT`).WithArgs(int64(99)).WillReturnError(pgx.ErrNoRows)
			},
			wantErr:   true,
			wantErrIs: domain.ErrProductNotFound,
		},
		{
			name:      "query error propagated",
			productID: 1,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`SELECT`).WithArgs(int64(1)).WillReturnError(errors.New("db error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock := newPool(t)
			tt.setup(mock)

			repo := repository.NewRewardedRepository(mock)
			got, err := repo.GetProduct(context.Background(), tt.productID)

			if tt.wantErr {
				if err == nil {
					t.Fatal("GetProduct() error = nil, want error")
				}

				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Errorf("GetProduct() error = %v, want %v", err, tt.wantErrIs)
				}

				return
			}

			if err != nil {
				t.Fatalf("GetProduct() unexpected error = %v", err)
			}

			if got.ID != tt.wantID {
				t.Errorf("product.ID = %d, want %d", got.ID, tt.wantID)
			}

			if got.Kind != tt.wantKind {
				t.Errorf("product.Kind = %q, want %q", got.Kind, tt.wantKind)
			}

			if got.IsFree != tt.wantIsFree {
				t.Errorf("product.IsFree = %v, want %v", got.IsFree, tt.wantIsFree)
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestRewardedRepository_GrantFreeDistraction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		userID    int64
		productID int64
		setup     func(m pgxmock.PgxPoolIface)
		wantQty   int
		wantErr   bool
	}{
		{
			// criterion: 1 — GrantFreeDistraction upserts and returns the incremented
			// quantity via RETURNING.
			name:      "success returns incremented quantity",
			userID:    1,
			productID: 10,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"quantity"}).AddRow(3)
				m.ExpectQuery(`INSERT INTO inventory`).
					WithArgs(int64(1), int64(10)).
					WillReturnRows(rows)
			},
			wantQty: 3,
		},
		{
			// criterion: 1 — the first grant for a (user, product) pair with no prior
			// row returns quantity 1.
			name:      "first grant returns quantity one",
			userID:    2,
			productID: 20,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"quantity"}).AddRow(1)
				m.ExpectQuery(`INSERT INTO inventory`).
					WithArgs(int64(2), int64(20)).
					WillReturnRows(rows)
			},
			wantQty: 1,
		},
		{
			name:      "query error propagated",
			userID:    1,
			productID: 10,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`INSERT INTO inventory`).
					WithArgs(int64(1), int64(10)).
					WillReturnError(errors.New("db error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock := newPool(t)
			tt.setup(mock)

			repo := repository.NewRewardedRepository(mock)
			got, err := repo.GrantFreeDistraction(context.Background(), tt.userID, tt.productID)

			if tt.wantErr {
				if err == nil {
					t.Fatal("GrantFreeDistraction() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("GrantFreeDistraction() unexpected error = %v", err)
			}

			if got != tt.wantQty {
				t.Errorf("GrantFreeDistraction() quantity = %d, want %d", got, tt.wantQty)
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}
