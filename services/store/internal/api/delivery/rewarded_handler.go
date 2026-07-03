package delivery

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/store/internal/api/domain"
	"github.com/pizdagladki/full/services/store/internal/api/service"
)

type rewardedHandler struct {
	rewardedSvc service.RewardedService
	logger      *zap.Logger
}

// NewRewardedHandler returns a RewardedHandler wired to the given service.
func NewRewardedHandler(rewardedSvc service.RewardedService, logger *zap.Logger) RewardedHandler {
	return &rewardedHandler{rewardedSvc: rewardedSvc, logger: logger}
}

type grantRewardedRequest struct {
	ProductID int64 `json:"product_id" validate:"required,min=1"`
}

type grantRewardedResponse struct {
	ProductID int64 `json:"product_id"`
	Quantity  int   `json:"quantity"`
}

// Grant handles POST /v1/store/rewarded/grant. Requires auth.
func (h *rewardedHandler) Grant(c echo.Context) error {
	userID, ok := c.Get(UserIDContextKey).(int64)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}

	var req grantRewardedRequest

	err := c.Bind(&req)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad request"})
	}

	err = c.Validate(&req)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad request"})
	}

	quantity, err := h.rewardedSvc.GrantFreeDistraction(c.Request().Context(), userID, req.ProductID)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrProductNotFound):
			return c.JSON(http.StatusNotFound, map[string]string{"error": "product not found"})
		case errors.Is(err, domain.ErrNotGrantable):
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "product is not a grantable free distraction"})
		case errors.Is(err, domain.ErrRateLimited):
			return c.JSON(http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
		default:
			h.logger.Error("grant free distraction", zap.Error(err))

			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		}
	}

	return c.JSON(http.StatusOK, grantRewardedResponse{
		ProductID: req.ProductID,
		Quantity:  quantity,
	})
}
