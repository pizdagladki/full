package delivery

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/ratings/internal/api/domain"
	"github.com/pizdagladki/full/services/ratings/internal/api/service"
)

type ratingsHandler struct {
	service service.RatingsService
	logger  *zap.Logger
}

// NewRatingsHandler builds a RatingsHandler backed by svc.
func NewRatingsHandler(svc service.RatingsService, logger *zap.Logger) RatingsHandler {
	return &ratingsHandler{service: svc, logger: logger}
}

// ─── Request / Response DTOs ─────────────────────────────────────────────────

// matchResultRequest is the POST /v1/matches/result body.
type matchResultRequest struct {
	WinnerID   int64  `json:"winner_id"  validate:"required"`
	LoserID    int64  `json:"loser_id"   validate:"required"`
	Mode       string `json:"mode"       validate:"required"`
	DurationMS *int   `json:"duration_ms"`
}

// playerRating is the per-player fragment in the response.
type playerRating struct {
	ELO         int `json:"elo"`
	Level       int `json:"level"`
	GamesPlayed int `json:"games_played"`
	ELODelta    int `json:"elo_delta"`
}

// matchResultResponse is the 200 body for POST /v1/matches/result.
type matchResultResponse struct {
	Winner playerRating `json:"winner"`
	Loser  playerRating `json:"loser"`
}

// ratingResponse is the 200 body for GET /v1/ratings/:user_id.
type ratingResponse struct {
	ELO         int `json:"elo"`
	Level       int `json:"level"`
	GamesPlayed int `json:"games_played"`
}

// ─── Handlers ────────────────────────────────────────────────────────────────

// PostMatchResult handles POST /v1/matches/result.
func (h *ratingsHandler) PostMatchResult(c echo.Context) error {
	var req matchResultRequest

	err := c.Bind(&req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	err = c.Validate(req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	result, err := h.service.ApplyMatchResult(c.Request().Context(), domain.MatchInput{
		WinnerID:   req.WinnerID,
		LoserID:    req.LoserID,
		Mode:       req.Mode,
		DurationMS: req.DurationMS,
	})
	if err != nil {
		if errors.Is(err, service.ErrSamePlayer) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}

		h.logger.Error("apply match result", zap.Error(err))

		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	return c.JSON(http.StatusOK, matchResultResponse{
		Winner: playerRating{
			ELO:         result.Winner.ELO,
			Level:       result.Winner.Level,
			GamesPlayed: result.Winner.GamesPlayed,
			ELODelta:    result.WinnerDelta,
		},
		Loser: playerRating{
			ELO:         result.Loser.ELO,
			Level:       result.Loser.Level,
			GamesPlayed: result.Loser.GamesPlayed,
			ELODelta:    result.LoserDelta,
		},
	})
}

// GetRating handles GET /v1/ratings/:user_id.
func (h *ratingsHandler) GetRating(c echo.Context) error {
	raw := c.Param("user_id")

	userID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || userID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "user_id must be a positive integer")
	}

	rating, err := h.service.GetRating(c.Request().Context(), userID)
	if err != nil {
		h.logger.Error("get rating", zap.Int64("user_id", userID), zap.Error(err))

		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	return c.JSON(http.StatusOK, ratingResponse{
		ELO:         rating.ELO,
		Level:       rating.Level,
		GamesPlayed: rating.GamesPlayed,
	})
}
