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

func newPool(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool() error = %v", err)
	}

	t.Cleanup(mock.Close)

	return mock
}

func TestPurchaseRepository_GetProduct(t *testing.T) {
	t.Parallel()

	cols := []string{"id", "kind", "tier", "name", "price_cents", "is_free"}

	tests := []struct {
		name      string
		productID int64
		setup     func(m pgxmock.PgxPoolIface)
		wantID    int64
		wantKind  string
		wantErr   bool
		wantErrIs error
	}{
		{
			// criterion: 1 — GetProduct returns product fields correctly when found
			name:      "found returns product",
			productID: 42,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(cols).
					AddRow(int64(42), "edit", nil, "Blur", 500, false)
				m.ExpectQuery(`SELECT`).WithArgs(int64(42)).WillReturnRows(rows)
			},
			wantID:   42,
			wantKind: "edit",
		},
		{
			// criterion: 1 — GetProduct returns ErrProductNotFound for pgx.ErrNoRows
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

			repo := repository.NewPurchaseRepository(mock)
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

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestPurchaseRepository_IsOwned(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		userID    int64
		productID int64
		setup     func(m pgxmock.PgxPoolIface)
		want      bool
		wantErr   bool
	}{
		{
			// criterion: 2 — IsOwned returns true when inventory quantity > 0
			name:      "owned returns true",
			userID:    1,
			productID: 10,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"exists"}).AddRow(true)
				m.ExpectQuery(`SELECT EXISTS`).WithArgs(int64(1), int64(10)).WillReturnRows(rows)
			},
			want: true,
		},
		{
			// criterion: 2 — IsOwned returns false when not in inventory
			name:      "not owned returns false",
			userID:    1,
			productID: 10,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"exists"}).AddRow(false)
				m.ExpectQuery(`SELECT EXISTS`).WithArgs(int64(1), int64(10)).WillReturnRows(rows)
			},
			want: false,
		},
		{
			name:      "query error propagated",
			userID:    1,
			productID: 10,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`SELECT EXISTS`).WithArgs(int64(1), int64(10)).WillReturnError(errors.New("db error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock := newPool(t)
			tt.setup(mock)

			repo := repository.NewPurchaseRepository(mock)
			got, err := repo.IsOwned(context.Background(), tt.userID, tt.productID)

			if tt.wantErr {
				if err == nil {
					t.Fatal("IsOwned() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("IsOwned() unexpected error = %v", err)
			}

			if got != tt.want {
				t.Errorf("IsOwned() = %v, want %v", got, tt.want)
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestPurchaseRepository_CreatePurchase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		purchase domain.Purchase
		setup    func(m pgxmock.PgxPoolIface)
		wantID   int64
		wantErr  bool
	}{
		{
			// criterion: 3 — CreatePurchase inserts record and returns ID
			name: "success returns id",
			purchase: domain.Purchase{
				UserID:      1,
				ProductID:   10,
				Provider:    domain.ProviderStripe,
				ProviderRef: "pi_test",
				AmountCents: 500,
				Status:      domain.PurchaseStatusPending,
			},
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"id"}).AddRow(int64(7))
				m.ExpectQuery(`INSERT INTO purchases`).
					WithArgs(int64(1), int64(10), domain.ProviderStripe, "pi_test", 500, domain.PurchaseStatusPending).
					WillReturnRows(rows)
			},
			wantID: 7,
		},
		{
			name: "insert error propagated",
			purchase: domain.Purchase{
				UserID:    1,
				ProductID: 10,
			},
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`INSERT INTO purchases`).WillReturnError(errors.New("db error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock := newPool(t)
			tt.setup(mock)

			repo := repository.NewPurchaseRepository(mock)
			gotID, err := repo.CreatePurchase(context.Background(), tt.purchase)

			if tt.wantErr {
				if err == nil {
					t.Fatal("CreatePurchase() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("CreatePurchase() unexpected error = %v", err)
			}

			if gotID != tt.wantID {
				t.Errorf("CreatePurchase() id = %d, want %d", gotID, tt.wantID)
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestPurchaseRepository_WebhookEventExists(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		eventID string
		setup   func(m pgxmock.PgxPoolIface)
		want    bool
		wantErr bool
	}{
		{
			// criterion: 4 — WebhookEventExists returns true for known event
			name:    "event exists returns true",
			eventID: "evt_known",
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"exists"}).AddRow(true)
				m.ExpectQuery(`SELECT EXISTS`).WithArgs("evt_known").WillReturnRows(rows)
			},
			want: true,
		},
		{
			// criterion: 4 — WebhookEventExists returns false for unknown event
			name:    "event not exists returns false",
			eventID: "evt_unknown",
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"exists"}).AddRow(false)
				m.ExpectQuery(`SELECT EXISTS`).WithArgs("evt_unknown").WillReturnRows(rows)
			},
			want: false,
		},
		{
			name:    "query error propagated",
			eventID: "evt_x",
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`SELECT EXISTS`).WithArgs("evt_x").WillReturnError(errors.New("db error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock := newPool(t)
			tt.setup(mock)

			repo := repository.NewPurchaseRepository(mock)
			got, err := repo.WebhookEventExists(context.Background(), tt.eventID)

			if tt.wantErr {
				if err == nil {
					t.Fatal("WebhookEventExists() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("WebhookEventExists() unexpected error = %v", err)
			}

			if got != tt.want {
				t.Errorf("WebhookEventExists() = %v, want %v", got, tt.want)
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestPurchaseRepository_ConfirmAndGrant(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		providerRef string
		eventID     string
		kind        string
		userID      int64
		productID   int64
		setup       func(m pgxmock.PgxPoolIface)
		wantErr     bool
	}{
		{
			// criterion: 5 — ConfirmAndGrant uses DO UPDATE for distraction (quantity increments)
			name:        "distraction increments quantity via DO UPDATE",
			providerRef: "pi_dist",
			eventID:     "evt_dist",
			kind:        domain.KindDistraction,
			userID:      1,
			productID:   10,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectBegin()
				m.ExpectExec(`UPDATE purchases`).
					WithArgs("evt_dist", "pi_dist").
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))
				m.ExpectExec(`INSERT INTO inventory`).
					WithArgs(int64(1), int64(10)).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))
				m.ExpectCommit()
			},
		},
		{
			// criterion: 5 — ConfirmAndGrant uses DO NOTHING for edit (no quantity increment)
			name:        "edit uses DO NOTHING on inventory insert",
			providerRef: "pi_edit",
			eventID:     "evt_edit",
			kind:        domain.KindEdit,
			userID:      2,
			productID:   20,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectBegin()
				m.ExpectExec(`UPDATE purchases`).
					WithArgs("evt_edit", "pi_edit").
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))
				m.ExpectExec(`INSERT INTO inventory`).
					WithArgs(int64(2), int64(20)).
					WillReturnResult(pgxmock.NewResult("INSERT", 0))
				m.ExpectCommit()
			},
		},
		{
			// criterion: 5 — ConfirmAndGrant runs in a single transaction (begin + commit)
			name:        "transaction commits on success",
			providerRef: "pi_tx",
			eventID:     "evt_tx",
			kind:        domain.KindDistraction,
			userID:      3,
			productID:   30,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectBegin()
				m.ExpectExec(`UPDATE purchases`).
					WithArgs("evt_tx", "pi_tx").
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))
				m.ExpectExec(`INSERT INTO inventory`).
					WithArgs(int64(3), int64(30)).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))
				m.ExpectCommit()
			},
		},
		{
			name:        "update error triggers rollback",
			providerRef: "pi_fail",
			eventID:     "evt_fail",
			kind:        domain.KindDistraction,
			userID:      1,
			productID:   10,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectBegin()
				m.ExpectExec(`UPDATE purchases`).
					WithArgs("evt_fail", "pi_fail").
					WillReturnError(errors.New("update failed"))
				m.ExpectRollback()
			},
			wantErr: true,
		},
		{
			name:        "inventory upsert error triggers rollback",
			providerRef: "pi_inv_fail",
			eventID:     "evt_inv_fail",
			kind:        domain.KindDistraction,
			userID:      1,
			productID:   10,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectBegin()
				m.ExpectExec(`UPDATE purchases`).
					WithArgs("evt_inv_fail", "pi_inv_fail").
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))
				m.ExpectExec(`INSERT INTO inventory`).
					WithArgs(int64(1), int64(10)).
					WillReturnError(errors.New("upsert failed"))
				m.ExpectRollback()
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock := newPool(t)
			tt.setup(mock)

			repo := repository.NewPurchaseRepository(mock)
			err := repo.ConfirmAndGrant(
				context.Background(),
				tt.providerRef, tt.eventID, tt.kind,
				tt.userID, tt.productID,
			)

			if tt.wantErr {
				if err == nil {
					t.Fatal("ConfirmAndGrant() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("ConfirmAndGrant() unexpected error = %v", err)
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestPurchaseRepository_FindByProviderRef(t *testing.T) {
	t.Parallel()

	cols := []string{"id", "user_id", "product_id", "provider", "provider_ref", "amount_cents", "status"}

	tests := []struct {
		name        string
		providerRef string
		setup       func(m pgxmock.PgxPoolIface)
		wantUserID  int64
		wantErr     bool
	}{
		{
			name:        "found returns purchase",
			providerRef: "pi_found",
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(cols).
					AddRow(int64(1), int64(5), int64(10), domain.ProviderStripe, "pi_found", 500, domain.PurchaseStatusPending)
				m.ExpectQuery(`SELECT`).WithArgs("pi_found").WillReturnRows(rows)
			},
			wantUserID: 5,
		},
		{
			name:        "not found returns error",
			providerRef: "pi_missing",
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`SELECT`).WithArgs("pi_missing").WillReturnError(pgx.ErrNoRows)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock := newPool(t)
			tt.setup(mock)

			repo := repository.NewPurchaseRepository(mock)
			got, err := repo.FindByProviderRef(context.Background(), tt.providerRef)

			if tt.wantErr {
				if err == nil {
					t.Fatal("FindByProviderRef() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("FindByProviderRef() unexpected error = %v", err)
			}

			if got.UserID != tt.wantUserID {
				t.Errorf("purchase.UserID = %d, want %d", got.UserID, tt.wantUserID)
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}
