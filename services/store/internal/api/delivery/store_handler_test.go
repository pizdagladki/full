package delivery_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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

func TestGetCatalog(t *testing.T) {
	t.Parallel()

	tier1 := 1
	allProducts := []domain.Product{
		{ID: 1, Kind: "distraction", Tier: &tier1, Name: "Spinner", PriceCents: 0, IsFree: true},
		{ID: 2, Kind: "edit", Name: "Blur", PriceCents: 100, IsFree: false},
	}

	tests := []struct {
		name       string
		queryKind  string
		setupSvc   func(m *svcmocks.MockCatalogService)
		wantStatus int
		wantLen    int
	}{
		{
			name:      "no kind filter returns all products",
			queryKind: "",
			setupSvc: func(m *svcmocks.MockCatalogService) {
				m.EXPECT().ListCatalog(gomock.Any(), (*string)(nil)).
					Return(allProducts, nil)
			},
			wantStatus: http.StatusOK,
			wantLen:    2,
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
			name:      "service error returns 500",
			queryKind: "",
			setupSvc: func(m *svcmocks.MockCatalogService) {
				m.EXPECT().ListCatalog(gomock.Any(), (*string)(nil)).
					Return(nil, errors.New("db down"))
			},
			wantStatus: http.StatusInternalServerError,
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

			if tt.wantStatus == http.StatusOK {
				var resp []map[string]any
				if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
					t.Fatalf("decode response: %v", err)
				}

				if len(resp) != tt.wantLen {
					t.Errorf("len(resp) = %d, want %d", len(resp), tt.wantLen)
				}

				// Verify non-null array (empty case).
				if resp == nil {
					t.Error("response is null, want []")
				}
			}
		})
	}
}

func TestGetInventory(t *testing.T) {
	t.Parallel()

	ownedItems := []domain.InventoryItem{
		{ProductID: 1, Quantity: 3},
		{ProductID: 5, Quantity: 1},
	}

	tests := []struct {
		name       string
		userID     any // the value stored in context; use int64 for valid, other type for missing
		setupSvc   func(m *svcmocks.MockInventoryService)
		wantStatus int
		wantLen    int
	}{
		{
			name:   "authenticated user with items",
			userID: int64(42),
			setupSvc: func(m *svcmocks.MockInventoryService) {
				m.EXPECT().ListInventory(gomock.Any(), int64(42)).
					Return(ownedItems, nil)
			},
			wantStatus: http.StatusOK,
			wantLen:    2,
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
			name:   "service error returns 500",
			userID: int64(1),
			setupSvc: func(m *svcmocks.MockInventoryService) {
				m.EXPECT().ListInventory(gomock.Any(), int64(1)).
					Return(nil, errors.New("db down"))
			},
			wantStatus: http.StatusInternalServerError,
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

			if tt.wantStatus == http.StatusOK {
				var resp []map[string]any
				if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
					t.Fatalf("decode response: %v", err)
				}

				if len(resp) != tt.wantLen {
					t.Errorf("len(resp) = %d, want %d", len(resp), tt.wantLen)
				}

				if resp == nil {
					t.Error("response is null, want []")
				}
			}
		})
	}
}
