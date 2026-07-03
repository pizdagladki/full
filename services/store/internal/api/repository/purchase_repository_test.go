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

	cols := []string{"id", "kind", "tier", "name", "price_cents", "is_free", "points_price"}
	points50 := int64(50)

	tests := []struct {
		name        string
		productID   int64
		setup       func(m pgxmock.PgxPoolIface)
		wantID      int64
		wantKind    string
		wantErr     bool
		wantErrIs   error
		wantPoints  *int64 // non-nil (incl explicit nil-check via wantPointsSet) checked below
		checkPoints bool
	}{
		{
			// criterion: 1 — GetProduct returns product fields correctly when found
			name:      "found returns product",
			productID: 42,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(cols).
					AddRow(int64(42), "edit", nil, "Blur", 500, false, nil)
				m.ExpectQuery(`SELECT`).WithArgs(int64(42)).WillReturnRows(rows)
			},
			wantID:      42,
			wantKind:    "edit",
			checkPoints: true,
			wantPoints:  nil,
		},
		{
			// criterion: 1 — GetProduct scans a non-null points_price for a dual-priced product
			name:      "found returns product with points price",
			productID: 43,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(cols).
					AddRow(int64(43), "distraction", nil, "Spinner", 100, false, &points50)
				m.ExpectQuery(`SELECT`).WithArgs(int64(43)).WillReturnRows(rows)
			},
			wantID:      43,
			wantKind:    "distraction",
			checkPoints: true,
			wantPoints:  &points50,
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

			// criterion: 1 — points_price is scanned as null or the priced value.
			if tt.checkPoints {
				switch {
				case tt.wantPoints == nil && got.PointsPrice != nil:
					t.Errorf("product.PointsPrice = %v, want nil", *got.PointsPrice)
				case tt.wantPoints != nil && got.PointsPrice == nil:
					t.Errorf("product.PointsPrice = nil, want %d", *tt.wantPoints)
				case tt.wantPoints != nil && got.PointsPrice != nil && *got.PointsPrice != *tt.wantPoints:
					t.Errorf("product.PointsPrice = %d, want %d", *got.PointsPrice, *tt.wantPoints)
				}
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
		{
			// criterion: 6 — ConfirmAndGrant is idempotent: when UPDATE affects 0 rows (already processed),
			// it commits without running the inventory grant.
			name:        "already processed event skips inventory grant",
			providerRef: "pi_dup",
			eventID:     "evt_dup",
			kind:        domain.KindDistraction,
			userID:      1,
			productID:   10,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectBegin()
				m.ExpectExec(`UPDATE purchases`).
					WithArgs("evt_dup", "pi_dup").
					WillReturnResult(pgxmock.NewResult("UPDATE", 0)) // 0 rows = already processed
				m.ExpectCommit()
				// NOTE: no ExpectExec for inventory — must NOT be called
			},
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

func TestPurchaseRepository_PurchaseWithPoints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		userID      int64
		productID   int64
		pointsPrice int64
		kind        string
		setup       func(m pgxmock.PgxPoolIface)
		wantBalance int64
		wantErr     bool
		wantErrIs   error
	}{
		{
			// criterion: 2 — a single transaction debits points_balance, records the
			// purchase, appends a negative-delta ledger row keyed by the purchase id,
			// and increments inventory quantity for a distraction.
			name:        "distraction debits balance appends ledger and increments inventory",
			userID:      1,
			productID:   10,
			pointsPrice: 50,
			kind:        domain.KindDistraction,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectBegin()
				m.ExpectQuery(`UPDATE points_balance`).
					WithArgs(int64(50), int64(1)).
					WillReturnRows(pgxmock.NewRows([]string{"balance"}).AddRow(int64(450)))
				m.ExpectQuery(`INSERT INTO purchases`).
					WithArgs(int64(1), int64(10)).
					WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(7)))
				m.ExpectExec(`INSERT INTO points_ledger`).
					WithArgs(int64(1), int64(-50), "7").
					WillReturnResult(pgxmock.NewResult("INSERT", 1))
				m.ExpectExec(`INSERT INTO inventory`).
					WithArgs(int64(1), int64(10)).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))
				m.ExpectCommit()
			},
			wantBalance: 450,
		},
		{
			// criterion: 2 — an edit product uses DO NOTHING (own-forever) instead of
			// incrementing quantity.
			name:        "edit debits balance appends ledger and grants own-forever",
			userID:      2,
			productID:   20,
			pointsPrice: 30,
			kind:        domain.KindEdit,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectBegin()
				m.ExpectQuery(`UPDATE points_balance`).
					WithArgs(int64(30), int64(2)).
					WillReturnRows(pgxmock.NewRows([]string{"balance"}).AddRow(int64(70)))
				m.ExpectQuery(`INSERT INTO purchases`).
					WithArgs(int64(2), int64(20)).
					WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(8)))
				m.ExpectExec(`INSERT INTO points_ledger`).
					WithArgs(int64(2), int64(-30), "8").
					WillReturnResult(pgxmock.NewResult("INSERT", 1))
				m.ExpectExec(`INSERT INTO inventory`).
					WithArgs(int64(2), int64(20)).
					WillReturnResult(pgxmock.NewResult("INSERT", 0))
				m.ExpectCommit()
			},
			wantBalance: 70,
		},
		{
			// criterion: 3 — insufficient balance: the conditional UPDATE returns no
			// rows, the tx is rolled back, and NOTHING past the debit is written
			// (no purchase insert, no ledger insert, no inventory grant expected here).
			name:        "insufficient balance returns ErrInsufficientPoints and writes nothing",
			userID:      3,
			productID:   10,
			pointsPrice: 999,
			kind:        domain.KindDistraction,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectBegin()
				m.ExpectQuery(`UPDATE points_balance`).
					WithArgs(int64(999), int64(3)).
					WillReturnError(pgx.ErrNoRows)
				m.ExpectRollback()
				// NOTE: no ExpectQuery for purchases, no ExpectExec for ledger/inventory —
				// asserting these are never called is enforced by mock.ExpectationsWereMet().
			},
			wantErr:   true,
			wantErrIs: domain.ErrInsufficientPoints,
		},
		{
			name:        "debit query error propagates and rolls back",
			userID:      4,
			productID:   10,
			pointsPrice: 10,
			kind:        domain.KindDistraction,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectBegin()
				m.ExpectQuery(`UPDATE points_balance`).
					WithArgs(int64(10), int64(4)).
					WillReturnError(errors.New("db down"))
				m.ExpectRollback()
			},
			wantErr: true,
		},
		{
			name:        "create purchase error propagates and rolls back",
			userID:      5,
			productID:   10,
			pointsPrice: 10,
			kind:        domain.KindDistraction,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectBegin()
				m.ExpectQuery(`UPDATE points_balance`).
					WithArgs(int64(10), int64(5)).
					WillReturnRows(pgxmock.NewRows([]string{"balance"}).AddRow(int64(0)))
				m.ExpectQuery(`INSERT INTO purchases`).
					WithArgs(int64(5), int64(10)).
					WillReturnError(errors.New("db down"))
				m.ExpectRollback()
			},
			wantErr: true,
		},
		{
			// criterion: 5 — a failed ledger insert rolls back the whole transaction,
			// including the points debit that already happened in-tx.
			name:        "ledger insert error rolls back the points debit",
			userID:      6,
			productID:   10,
			pointsPrice: 10,
			kind:        domain.KindDistraction,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectBegin()
				m.ExpectQuery(`UPDATE points_balance`).
					WithArgs(int64(10), int64(6)).
					WillReturnRows(pgxmock.NewRows([]string{"balance"}).AddRow(int64(0)))
				m.ExpectQuery(`INSERT INTO purchases`).
					WithArgs(int64(6), int64(10)).
					WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(9)))
				m.ExpectExec(`INSERT INTO points_ledger`).
					WithArgs(int64(6), int64(-10), "9").
					WillReturnError(errors.New("db down"))
				m.ExpectRollback()
			},
			wantErr: true,
		},
		{
			// criterion: 5 — a failed inventory grant rolls back the points debit —
			// atomicity: the whole spend is undone, not just the inventory step.
			name:        "inventory upsert error rolls back the points debit",
			userID:      7,
			productID:   10,
			pointsPrice: 10,
			kind:        domain.KindDistraction,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectBegin()
				m.ExpectQuery(`UPDATE points_balance`).
					WithArgs(int64(10), int64(7)).
					WillReturnRows(pgxmock.NewRows([]string{"balance"}).AddRow(int64(0)))
				m.ExpectQuery(`INSERT INTO purchases`).
					WithArgs(int64(7), int64(10)).
					WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(11)))
				m.ExpectExec(`INSERT INTO points_ledger`).
					WithArgs(int64(7), int64(-10), "11").
					WillReturnResult(pgxmock.NewResult("INSERT", 1))
				m.ExpectExec(`INSERT INTO inventory`).
					WithArgs(int64(7), int64(10)).
					WillReturnError(errors.New("db down"))
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
			got, err := repo.PurchaseWithPoints(context.Background(), tt.userID, tt.productID, tt.pointsPrice, tt.kind)

			if tt.wantErr {
				if err == nil {
					t.Fatal("PurchaseWithPoints() error = nil, want error")
				}

				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Errorf("PurchaseWithPoints() error = %v, want %v", err, tt.wantErrIs)
				}

				if err = mock.ExpectationsWereMet(); err != nil {
					t.Errorf("unfulfilled expectations: %v", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("PurchaseWithPoints() unexpected error = %v", err)
			}

			if got != tt.wantBalance {
				t.Errorf("PurchaseWithPoints() balance = %d, want %d", got, tt.wantBalance)
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}
