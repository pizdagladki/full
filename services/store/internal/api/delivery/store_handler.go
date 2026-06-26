package delivery

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/store/internal/api/domain"
	"github.com/pizdagladki/full/services/store/internal/api/service"
)

// productResponse is the JSON representation of a catalog product.
type productResponse struct {
	ID         int64  `json:"id"`
	Kind       string `json:"kind"`
	Tier       *int   `json:"tier"`
	Name       string `json:"name"`
	PriceCents int    `json:"price_cents"`
	IsFree     bool   `json:"is_free"`
}

// inventoryItemResponse is the JSON representation of an inventory entry.
type inventoryItemResponse struct {
	ProductID int64 `json:"product_id"`
	Quantity  int   `json:"quantity"`
}

type storeHandler struct {
	catalogSvc   service.CatalogService
	inventorySvc service.InventoryService
	logger       *zap.Logger
}

// NewStoreHandler returns a StoreHandler wired to the given services.
func NewStoreHandler(
	catalogSvc service.CatalogService,
	inventorySvc service.InventoryService,
	logger *zap.Logger,
) StoreHandler {
	return &storeHandler{
		catalogSvc:   catalogSvc,
		inventorySvc: inventorySvc,
		logger:       logger,
	}
}

func (h *storeHandler) GetCatalog(c echo.Context) error {
	var kindPtr *string

	if k := c.QueryParam("kind"); k != "" {
		if !domain.ValidKind(k) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid kind"})
		}

		kindPtr = &k
	}

	products, err := h.catalogSvc.ListCatalog(c.Request().Context(), kindPtr)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidKind) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid kind"})
		}

		h.logger.Error("list catalog", zap.Error(err))

		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	resp := make([]productResponse, 0, len(products))

	for _, p := range products {
		resp = append(resp, productResponse{
			ID:         p.ID,
			Kind:       p.Kind,
			Tier:       p.Tier,
			Name:       p.Name,
			PriceCents: p.PriceCents,
			IsFree:     p.IsFree,
		})
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *storeHandler) GetInventory(c echo.Context) error {
	userID, ok := c.Get(UserIDContextKey).(int64)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}

	items, err := h.inventorySvc.ListInventory(c.Request().Context(), userID)
	if err != nil {
		h.logger.Error("list inventory", zap.Error(err))

		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	resp := make([]inventoryItemResponse, 0, len(items))

	for _, item := range items {
		resp = append(resp, inventoryItemResponse{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
		})
	}

	return c.JSON(http.StatusOK, resp)
}
