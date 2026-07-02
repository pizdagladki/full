package delivery

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/koth/internal/api/domain"
	"github.com/pizdagladki/full/services/koth/internal/api/service"
)

type rankHandler struct {
	rankSvc service.RankService
	logger  *zap.Logger
}

// NewRankHandler returns a RankHandler wired to the given service.
func NewRankHandler(rankSvc service.RankService, logger *zap.Logger) RankHandler {
	return &rankHandler{rankSvc: rankSvc, logger: logger}
}

type submitAttemptRequest struct {
	HeldMs int `json:"held_ms" validate:"required,gt=0"`
}

type submitAttemptResponse struct {
	AchievedRank int  `json:"achieved_rank"`
	CurrentRank  int  `json:"current_rank"`
	NewlyReached bool `json:"newly_reached"`
}

type meResponse struct {
	CurrentRank  int `json:"current_rank"`
	NextTargetMs int `json:"next_target_ms"`
}

type rankCountResponse struct {
	Rank  int `json:"rank"`
	Count int `json:"count"`
}

// SubmitAttempt handles POST /v1/koth/ranked/attempt. Requires auth.
func (h *rankHandler) SubmitAttempt(c echo.Context) error {
	userID, ok := c.Get(UserIDContextKey).(int64)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}

	var req submitAttemptRequest

	err := c.Bind(&req)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad request"})
	}

	err = c.Validate(&req)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad request"})
	}

	result, err := h.rankSvc.SubmitAttempt(c.Request().Context(), userID, req.HeldMs)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidHoldMs) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "held_ms must be positive"})
		}

		h.logger.Error("submit attempt", zap.Error(err))

		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	return c.JSON(http.StatusOK, submitAttemptResponse{
		AchievedRank: result.AchievedRank,
		CurrentRank:  result.CurrentRank,
		NewlyReached: result.NewlyReached,
	})
}

// Me handles GET /v1/koth/ranked/me. Requires auth.
func (h *rankHandler) Me(c echo.Context) error {
	userID, ok := c.Get(UserIDContextKey).(int64)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}

	result, err := h.rankSvc.Me(c.Request().Context(), userID)
	if err != nil {
		h.logger.Error("me", zap.Error(err))

		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	return c.JSON(http.StatusOK, meResponse{
		CurrentRank:  result.CurrentRank,
		NextTargetMs: result.NextTargetMs,
	})
}

// Leaderboard handles GET /v1/koth/ranked/leaderboard. Public — no auth.
func (h *rankHandler) Leaderboard(c echo.Context) error {
	counts, err := h.rankSvc.Leaderboard(c.Request().Context())
	if err != nil {
		h.logger.Error("leaderboard", zap.Error(err))

		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	resp := make([]rankCountResponse, 0, len(counts))

	for _, rc := range counts {
		resp = append(resp, rankCountResponse{Rank: rc.Rank, Count: rc.Count})
	}

	return c.JSON(http.StatusOK, resp)
}
