package delivery

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/store/internal/api/domain"
	"github.com/pizdagladki/full/services/store/internal/api/service"
)

type pointsHandler struct {
	pointsSvc service.PointsService
	logger    *zap.Logger
}

// NewPointsHandler returns a PointsHandler wired to the given service.
func NewPointsHandler(pointsSvc service.PointsService, logger *zap.Logger) PointsHandler {
	return &pointsHandler{pointsSvc: pointsSvc, logger: logger}
}

type creditPointsRequest struct {
	UserID int64  `json:"user_id" validate:"required,min=1"`
	Reason string `json:"reason"`
	RefID  string `json:"ref_id"`
	Delta  int64  `json:"delta"`
}

type balanceResponse struct {
	Balance int64 `json:"balance"`
}

// Credit handles POST /v1/points/credit. This is a server-to-server endpoint
// — no session auth.
func (h *pointsHandler) Credit(c echo.Context) error {
	var req creditPointsRequest

	err := c.Bind(&req)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad request"})
	}

	err = c.Validate(&req)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad request"})
	}

	balance, err := h.pointsSvc.Credit(c.Request().Context(), domain.PointsCredit{
		UserID: req.UserID,
		Reason: req.Reason,
		RefID:  req.RefID,
		Delta:  req.Delta,
	})
	if err != nil {
		if errors.Is(err, domain.ErrInvalidCredit) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid credit"})
		}

		h.logger.Error("credit points", zap.Error(err))

		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	return c.JSON(http.StatusOK, balanceResponse{Balance: balance})
}

// GetBalance handles GET /v1/points/balance. Requires auth — RequireAuth
// enforces the 401 before this handler runs.
func (h *pointsHandler) GetBalance(c echo.Context) error {
	userID, ok := c.Get(UserIDContextKey).(int64)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}

	balance, err := h.pointsSvc.GetBalance(c.Request().Context(), userID)
	if err != nil {
		h.logger.Error("get points balance", zap.Error(err))

		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	return c.JSON(http.StatusOK, balanceResponse{Balance: balance})
}
