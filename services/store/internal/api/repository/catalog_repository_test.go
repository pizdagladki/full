package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pashagolub/pgxmock/v4"

	"github.com/pizdagladki/full/services/store/internal/api/domain"
	"github.com/pizdagladki/full/services/store/internal/api/repository"
)

func ptr[T any](v T) *T { return &v }

func TestCatalogRepository_ListProducts(t *testing.T) {
	t.Parallel()

	cols := []string{"id", "kind", "tier", "name", "price_cents", "is_free", "points_price"}
	tier1 := 1
	points50 := int64(50)

	tests := []struct {
		name         string
		kind         *string
		setup        func(m pgxmock.PgxPoolIface)
		wantLen      int
		wantErr      bool
		wantProducts []domain.Product // non-nil: assert scanned field values exactly (criterion: 1)
	}{
		{
			// criterion: 1 — "all products returned when kind is nil" asserts scanned field values:
			// id, kind, tier (nil for edit, non-nil for distraction), name, price_cents, is_free,
			// and points_price (non-nil for the priced product, null for the money-only one).
			name: "all products returned when kind is nil",
			kind: nil,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(cols).
					AddRow(int64(1), "distraction", &tier1, "Spinner", 0, true, &points50).
					AddRow(int64(2), "edit", nil, "Blur", 100, false, nil)
				m.ExpectQuery(`SELECT`).WillReturnRows(rows)
			},
			wantLen: 2,
			wantProducts: []domain.Product{
				{ID: 1, Kind: "distraction", Tier: ptr(1), Name: "Spinner", PriceCents: 0, IsFree: true, PointsPrice: ptr(int64(50))},
				{ID: 2, Kind: "edit", Tier: nil, Name: "Blur", PriceCents: 100, IsFree: false, PointsPrice: nil},
			},
		},
		{
			name: "filter by kind=distraction returns only matching rows",
			kind: ptr("distraction"),
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(cols).
					AddRow(int64(1), "distraction", &tier1, "Spinner", 0, true, &points50)
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

			// criterion: 1 — assert scanned field values including Tier nil/non-nil.
			for i, want := range tt.wantProducts {
				if i >= len(got) {
					t.Errorf("got[%d] missing, have only %d items", i, len(got))
					break
				}

				p := got[i]

				if p.ID != want.ID {
					t.Errorf("products[%d].ID = %d, want %d", i, p.ID, want.ID)
				}

				if p.Kind != want.Kind {
					t.Errorf("products[%d].Kind = %q, want %q", i, p.Kind, want.Kind)
				}

				switch {
				case want.Tier == nil && p.Tier != nil:
					t.Errorf("products[%d].Tier = %v, want nil", i, *p.Tier)
				case want.Tier != nil && p.Tier == nil:
					t.Errorf("products[%d].Tier = nil, want %d", i, *want.Tier)
				case want.Tier != nil && p.Tier != nil && *p.Tier != *want.Tier:
					t.Errorf("products[%d].Tier = %d, want %d", i, *p.Tier, *want.Tier)
				}

				if p.Name != want.Name {
					t.Errorf("products[%d].Name = %q, want %q", i, p.Name, want.Name)
				}

				if p.PriceCents != want.PriceCents {
					t.Errorf("products[%d].PriceCents = %d, want %d", i, p.PriceCents, want.PriceCents)
				}

				if p.IsFree != want.IsFree {
					t.Errorf("products[%d].IsFree = %v, want %v", i, p.IsFree, want.IsFree)
				}

				// criterion: 1 — points_price is scanned as null (money-only) or the priced value.
				switch {
				case want.PointsPrice == nil && p.PointsPrice != nil:
					t.Errorf("products[%d].PointsPrice = %v, want nil", i, *p.PointsPrice)
				case want.PointsPrice != nil && p.PointsPrice == nil:
					t.Errorf("products[%d].PointsPrice = nil, want %d", i, *want.PointsPrice)
				case want.PointsPrice != nil && p.PointsPrice != nil && *p.PointsPrice != *want.PointsPrice:
					t.Errorf("products[%d].PointsPrice = %d, want %d", i, *p.PointsPrice, *want.PointsPrice)
				}
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled pgxmock expectations: %v", err)
			}
		})
	}
}
