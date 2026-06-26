package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pashagolub/pgxmock/v4"

	"github.com/pizdagladki/full/services/store/internal/api/domain"
	"github.com/pizdagladki/full/services/store/internal/api/repository"
)

func TestInventoryRepository_ListByUser(t *testing.T) {
	t.Parallel()

	cols := []string{"product_id", "quantity"}

	// whereUserIDRe is a regexp that pins the WHERE user_id = $1 clause in the
	// query string.  pgxmock matches ExpectQuery patterns as regexps, so we
	// escape the SQL metacharacters that appear in the real query text.
	// This is the Blocker-3 assertion: a repo that omits the WHERE clause will
	// not match and the expectation will fail.
	const whereUserIDRe = `WHERE user_id = \$1`

	tests := []struct {
		name      string
		userID    int64
		setup     func(m pgxmock.PgxPoolIface)
		wantLen   int
		wantErr   bool
		wantItems []domain.InventoryItem // non-nil: assert scanned product_id and quantity (criterion: 2)
	}{
		{
			// criterion: 2 — "owned items returned" asserts exact product_id and quantity values.
			// criterion: 3 — ExpectQuery pins WHERE user_id = $1 so a missing filter fails.
			name:   "owned items returned",
			userID: 42,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(cols).
					AddRow(int64(1), 3).
					AddRow(int64(5), 1)
				m.ExpectQuery(whereUserIDRe).WithArgs(int64(42)).WillReturnRows(rows)
			},
			wantLen: 2,
			wantItems: []domain.InventoryItem{
				{ProductID: 1, Quantity: 3},
				{ProductID: 5, Quantity: 1},
			},
		},
		{
			// criterion: 3 — WHERE user_id clause is also exercised on the empty-result path.
			name:   "empty inventory returns empty slice not nil",
			userID: 99,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(cols)
				m.ExpectQuery(whereUserIDRe).WithArgs(int64(99)).WillReturnRows(rows)
			},
			wantLen: 0,
		},
		{
			// criterion: 3 — WHERE user_id clause is also exercised on the error path.
			name:   "query error is propagated",
			userID: 1,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(whereUserIDRe).WithArgs(int64(1)).
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
				m.ExpectQuery(whereUserIDRe).WithArgs(int64(2)).WillReturnRows(rows)
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

			// criterion: 2 — assert exact product_id and quantity for each returned item.
			for i, want := range tt.wantItems {
				if i >= len(got) {
					t.Errorf("got[%d] missing, have only %d items", i, len(got))
					break
				}

				item := got[i]

				if item.ProductID != want.ProductID {
					t.Errorf("items[%d].ProductID = %d, want %d", i, item.ProductID, want.ProductID)
				}

				if item.Quantity != want.Quantity {
					t.Errorf("items[%d].Quantity = %d, want %d", i, item.Quantity, want.Quantity)
				}
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled pgxmock expectations: %v", err)
			}
		})
	}
}
