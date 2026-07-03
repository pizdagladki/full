package delivery_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/store/internal/api/delivery"
	"github.com/pizdagladki/full/services/store/internal/api/domain"
	svcmocks "github.com/pizdagladki/full/services/store/internal/api/service/mocks"
)

func setupEcho(t *testing.T) *echo.Echo {
	t.Helper()

	e := echo.New()
	e.HideBanner = true

	return e
}

// wantProduct pins the expected JSON field values for a catalog product response.
type wantProduct struct {
	id          float64 // JSON numbers decode as float64
	kind        string
	tier        *float64 // nil means the JSON field should be null
	name        string
	priceCents  float64
	isFree      bool
	pointsPrice *float64 // nil means the JSON field should be null (money-only)
}

func assertProduct(t *testing.T, got map[string]any, want wantProduct) {
	t.Helper()

	if v, _ := got["id"].(float64); v != want.id {
		t.Errorf("product id = %v, want %v", v, want.id)
	}

	if v, _ := got["kind"].(string); v != want.kind {
		t.Errorf("product kind = %q, want %q", v, want.kind)
	}

	if want.tier == nil {
		if got["tier"] != nil {
			t.Errorf("product tier = %v, want null", got["tier"])
		}
	} else {
		v, _ := got["tier"].(float64)
		if v != *want.tier {
			t.Errorf("product tier = %v, want %v", v, *want.tier)
		}
	}

	if v, _ := got["name"].(string); v != want.name {
		t.Errorf("product name = %q, want %q", v, want.name)
	}

	if v, _ := got["price_cents"].(float64); v != want.priceCents {
		t.Errorf("product price_cents = %v, want %v", v, want.priceCents)
	}

	if v, _ := got["is_free"].(bool); v != want.isFree {
		t.Errorf("product is_free = %v, want %v", v, want.isFree)
	}

	// criterion: 1 — points_price is present and null for money-only products,
	// or the numeric price for dual-priced ones.
	if want.pointsPrice == nil {
		if got["points_price"] != nil {
			t.Errorf("product points_price = %v, want null", got["points_price"])
		}
	} else {
		v, _ := got["points_price"].(float64)
		if v != *want.pointsPrice {
			t.Errorf("product points_price = %v, want %v", v, *want.pointsPrice)
		}
	}
}

func ptrF(v float64) *float64 { return &v }

func TestGetCatalog(t *testing.T) {
	t.Parallel()

	tier1 := 1
	points50 := int64(50)
	allProducts := []domain.Product{
		{ID: 1, Kind: "distraction", Tier: &tier1, Name: "Spinner", PriceCents: 0, IsFree: true, PointsPrice: &points50},
		{ID: 2, Kind: "edit", Name: "Blur", PriceCents: 100, IsFree: false, PointsPrice: nil},
	}

	tests := []struct {
		name            string
		queryKind       string
		setupSvc        func(m *svcmocks.MockCatalogService)
		wantStatus      int
		wantLen         int
		wantBodyExclude string        // substring that must NOT appear in the error body
		wantProducts    []wantProduct // non-nil: assert each product's full field set (criterion: 1)
	}{
		{
			// criterion: 1 — "no kind filter returns all products" asserts full field contracts
			// including tier=null for edit and tier=1 for distraction, is_free, price_cents, name, id.
			name:      "no kind filter returns all products",
			queryKind: "",
			setupSvc: func(m *svcmocks.MockCatalogService) {
				m.EXPECT().ListCatalog(gomock.Any(), (*string)(nil)).
					Return(allProducts, nil)
			},
			wantStatus: http.StatusOK,
			wantLen:    2,
			wantProducts: []wantProduct{
				{id: 1, kind: "distraction", tier: ptrF(1), name: "Spinner", priceCents: 0, isFree: true, pointsPrice: ptrF(50)},
				{id: 2, kind: "edit", tier: nil, name: "Blur", priceCents: 100, isFree: false, pointsPrice: nil},
			},
		},
		{
			name:      "valid kind filter returns matching products",
			queryKind: "distraction",
			setupSvc: func(m *svcmocks.MockCatalogService) {
				m.EXPECT().ListCatalog(gomock.Any(), gomock.Any()).
					Return(allProducts[:1], nil)
			},
			wantStatus: http.StatusOK,
			wantLen:    1,
		},
		{
			name:       "invalid kind returns 400",
			queryKind:  "bogus",
			setupSvc:   func(_ *svcmocks.MockCatalogService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:      "empty catalog returns empty array not null",
			queryKind: "",
			setupSvc: func(m *svcmocks.MockCatalogService) {
				m.EXPECT().ListCatalog(gomock.Any(), (*string)(nil)).
					Return([]domain.Product{}, nil)
			},
			wantStatus: http.StatusOK,
			wantLen:    0,
		},
		{
			name:      "service error returns 500 with generic body not internal detail",
			queryKind: "",
			setupSvc: func(m *svcmocks.MockCatalogService) {
				m.EXPECT().ListCatalog(gomock.Any(), (*string)(nil)).
					Return(nil, errors.New("db down"))
			},
			wantStatus:      http.StatusInternalServerError,
			wantBodyExclude: "db down",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			catalogMock := svcmocks.NewMockCatalogService(ctrl)
			inventoryMock := svcmocks.NewMockInventoryService(ctrl)
			tt.setupSvc(catalogMock)

			h := delivery.NewStoreHandler(catalogMock, inventoryMock, zap.NewNop())

			e := setupEcho(t)
			url := "/v1/store/catalog"
			if tt.queryKind != "" {
				url += "?kind=" + tt.queryKind
			}

			req := httptest.NewRequest(http.MethodGet, url, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			_ = h.GetCatalog(c)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			body := rec.Body.String()

			if tt.wantBodyExclude != "" && strings.Contains(body, tt.wantBodyExclude) {
				t.Errorf("body must not contain internal detail %q but got: %s", tt.wantBodyExclude, body)
			}

			if tt.wantStatus == http.StatusOK {
				var resp []map[string]any
				if err := json.Unmarshal([]byte(body), &resp); err != nil {
					t.Fatalf("decode response: %v", err)
				}

				if len(resp) != tt.wantLen {
					t.Errorf("len(resp) = %d, want %d", len(resp), tt.wantLen)
				}

				// Verify non-null array (empty case).
				if resp == nil {
					t.Error("response is null, want []")
				}

				// criterion: 1 — assert full field set for each product.
				for i, want := range tt.wantProducts {
					if i >= len(resp) {
						t.Errorf("resp[%d] missing, have only %d items", i, len(resp))
						break
					}

					assertProduct(t, resp[i], want)
				}
			}
		})
	}
}

// wantInventoryItem pins the expected JSON field values for an inventory item response.
type wantInventoryItem struct {
	productID float64
	quantity  float64
}

func assertInventoryItem(t *testing.T, got map[string]any, want wantInventoryItem) {
	t.Helper()

	if v, _ := got["product_id"].(float64); v != want.productID {
		t.Errorf("item product_id = %v, want %v", v, want.productID)
	}

	if v, _ := got["quantity"].(float64); v != want.quantity {
		t.Errorf("item quantity = %v, want %v", v, want.quantity)
	}
}

func TestGetInventory(t *testing.T) {
	t.Parallel()

	ownedItems := []domain.InventoryItem{
		{ProductID: 1, Quantity: 3},
		{ProductID: 5, Quantity: 1},
	}

	tests := []struct {
		name            string
		userID          any // the value stored in context; use int64 for valid, other type for missing
		setupSvc        func(m *svcmocks.MockInventoryService)
		wantStatus      int
		wantLen         int
		wantBodyExclude string              // substring that must NOT appear in the error body
		wantItems       []wantInventoryItem // non-nil: assert each item's product_id and quantity (criterion: 2)
	}{
		{
			// criterion: 2 — "authenticated user with items" asserts exact product_id and quantity values.
			name:   "authenticated user with items",
			userID: int64(42),
			setupSvc: func(m *svcmocks.MockInventoryService) {
				m.EXPECT().ListInventory(gomock.Any(), int64(42)).
					Return(ownedItems, nil)
			},
			wantStatus: http.StatusOK,
			wantLen:    2,
			wantItems: []wantInventoryItem{
				{productID: 1, quantity: 3},
				{productID: 5, quantity: 1},
			},
		},
		{
			name:   "authenticated user with empty inventory",
			userID: int64(99),
			setupSvc: func(m *svcmocks.MockInventoryService) {
				m.EXPECT().ListInventory(gomock.Any(), int64(99)).
					Return([]domain.InventoryItem{}, nil)
			},
			wantStatus: http.StatusOK,
			wantLen:    0,
		},
		{
			name:       "no user id in context returns 401",
			userID:     nil, // missing - context key not set
			setupSvc:   func(_ *svcmocks.MockInventoryService) {},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:   "service error returns 500 with generic body not internal detail",
			userID: int64(1),
			setupSvc: func(m *svcmocks.MockInventoryService) {
				m.EXPECT().ListInventory(gomock.Any(), int64(1)).
					Return(nil, errors.New("db down"))
			},
			wantStatus:      http.StatusInternalServerError,
			wantBodyExclude: "db down",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			catalogMock := svcmocks.NewMockCatalogService(ctrl)
			inventoryMock := svcmocks.NewMockInventoryService(ctrl)
			tt.setupSvc(inventoryMock)

			h := delivery.NewStoreHandler(catalogMock, inventoryMock, zap.NewNop())

			e := setupEcho(t)
			req := httptest.NewRequest(http.MethodGet, "/v1/store/inventory", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			if tt.userID != nil {
				c.Set(delivery.UserIDContextKey, tt.userID)
			}

			_ = h.GetInventory(c)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			body := rec.Body.String()

			if tt.wantBodyExclude != "" && strings.Contains(body, tt.wantBodyExclude) {
				t.Errorf("body must not contain internal detail %q but got: %s", tt.wantBodyExclude, body)
			}

			if tt.wantStatus == http.StatusOK {
				var resp []map[string]any
				if err := json.Unmarshal([]byte(body), &resp); err != nil {
					t.Fatalf("decode response: %v", err)
				}

				if len(resp) != tt.wantLen {
					t.Errorf("len(resp) = %d, want %d", len(resp), tt.wantLen)
				}

				if resp == nil {
					t.Error("response is null, want []")
				}

				// criterion: 2 — assert exact product_id and quantity for each item.
				for i, want := range tt.wantItems {
					if i >= len(resp) {
						t.Errorf("resp[%d] missing, have only %d items", i, len(resp))
						break
					}

					assertInventoryItem(t, resp[i], want)
				}
			}
		})
	}
}
