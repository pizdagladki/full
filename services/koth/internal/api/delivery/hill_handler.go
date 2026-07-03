package delivery

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/koth/internal/api/domain"
	"github.com/pizdagladki/full/services/koth/internal/api/repository"
	"github.com/pizdagladki/full/services/koth/internal/api/service"
)

type hillHandler struct {
	hillSvc service.HillService
	logger  *zap.Logger
}

// NewHillHandler returns a HillHandler wired to the given service.
func NewHillHandler(hillSvc service.HillService, logger *zap.Logger) HillHandler {
	return &hillHandler{hillSvc: hillSvc, logger: logger}
}

type kingResponse struct {
	UserID    int64  `json:"user_id"`
	ClipID    string `json:"clip_id"`
	BlinkTsMs int    `json:"blink_ts_ms"`
}

type challengeRequest struct {
	SurvivedMs int    `json:"survived_ms" validate:"gte=0"`
	NewClipID  string `json:"new_clip_id" validate:"required"`
}

type challengeResponse struct {
	Won  bool         `json:"won"`
	King kingResponse `json:"king"`
}

func toKingResponse(king domain.KingReign) kingResponse {
	return kingResponse{
		UserID:    king.UserID,
		ClipID:    king.ClipID,
		BlinkTsMs: king.BlinkTsMs,
	}
}

// CurrentKing handles GET /v1/koth/hills/:hill_type/king. Public — no auth.
func (h *hillHandler) CurrentKing(c echo.Context) error {
	king, err := h.hillSvc.CurrentKing(c.Request().Context(), c.Param("hill_type"))
	if err != nil {
		if errors.Is(err, domain.ErrInvalidHillType) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid hill_type"})
		}

		if errors.Is(err, repository.ErrHillNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "hill not seeded"})
		}

		h.logger.Error("current king", zap.Error(err))

		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	return c.JSON(http.StatusOK, toKingResponse(king))
}

// Challenge handles POST /v1/koth/hills/:hill_type/challenge. Requires auth.
func (h *hillHandler) Challenge(c echo.Context) error {
	userID, ok := c.Get(UserIDContextKey).(int64)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}

	var req challengeRequest

	err := c.Bind(&req)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad request"})
	}

	err = c.Validate(&req)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad request"})
	}

	outcome, err := h.hillSvc.Challenge(
		c.Request().Context(), c.Param("hill_type"), userID, req.SurvivedMs, req.NewClipID,
	)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidHillType) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid hill_type"})
		}

		if errors.Is(err, repository.ErrHillNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "hill not seeded"})
		}

		h.logger.Error("challenge", zap.Error(err))

		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	return c.JSON(http.StatusOK, challengeResponse{
		Won:  outcome.Won,
		King: toKingResponse(outcome.King),
	})
}
