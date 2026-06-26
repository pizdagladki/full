package delivery

import (
	"errors"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/store/internal/api/domain"
	"github.com/pizdagladki/full/services/store/internal/api/service"
)

type purchaseHandler struct {
	purchaseSvc service.PurchaseService
	logger      *zap.Logger
}

// NewPurchaseHandler returns a PurchaseHandler wired to the given service.
func NewPurchaseHandler(purchaseSvc service.PurchaseService, logger *zap.Logger) PurchaseHandler {
	return &purchaseHandler{purchaseSvc: purchaseSvc, logger: logger}
}

type createPurchaseRequest struct {
	ProductID int64 `json:"product_id" validate:"required,min=1"`
}

type createPurchaseResponse struct {
	ClientSecret string `json:"client_secret"`
	ProductID    int64  `json:"product_id"`
}

// CreatePurchase handles POST /v1/store/purchase. Requires auth.
func (h *purchaseHandler) CreatePurchase(c echo.Context) error {
	userID, ok := c.Get(UserIDContextKey).(int64)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}

	var req createPurchaseRequest

	err := c.Bind(&req)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad request"})
	}

	err = c.Validate(&req)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad request"})
	}

	clientSecret, err := h.purchaseSvc.InitiatePurchase(c.Request().Context(), userID, req.ProductID)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrProductNotFound):
			return c.JSON(http.StatusNotFound, map[string]string{"error": "product not found"})
		case errors.Is(err, domain.ErrAlreadyOwned):
			return c.JSON(http.StatusConflict, map[string]string{"error": "already owned"})
		default:
			h.logger.Error("initiate purchase", zap.Error(err))

			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		}
	}

	return c.JSON(http.StatusCreated, createPurchaseResponse{
		ClientSecret: clientSecret,
		ProductID:    req.ProductID,
	})
}

// StripeWebhook handles POST /v1/store/stripe/webhook. Public endpoint — no auth.
// The raw body must be read before any binding so the Stripe signature can be verified.
func (h *purchaseHandler) StripeWebhook(c echo.Context) error {
	payload, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad request"})
	}

	sigHeader := c.Request().Header.Get("Stripe-Signature")

	err = h.purchaseSvc.HandleWebhook(c.Request().Context(), payload, sigHeader)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidWebhook) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid webhook"})
		}

		h.logger.Error("handle webhook", zap.Error(err))

		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
