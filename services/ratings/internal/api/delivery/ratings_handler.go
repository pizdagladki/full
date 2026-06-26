package delivery

import (
	"errors"
	"net/http"
	"strconv"
	"time"

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
	WinnerID   int64  `json:"winner_id"  validate:"required,gt=0"`
	LoserID    int64  `json:"loser_id"   validate:"required,gt=0"`
	Mode       string `json:"mode"       validate:"required,max=64"`
	DurationMS *int   `json:"duration_ms" validate:"omitempty,min=0"`
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

// matchHistoryItemResponse is a single entry in GET /v1/matches/history.
type matchHistoryItemResponse struct {
	MatchID    int64     `json:"match_id"`
	OpponentID int64     `json:"opponent_id"`
	Result     string    `json:"result"`
	Mode       string    `json:"mode"`
	ELODelta   int       `json:"elo_delta"`
	DurationMS *int      `json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at"`
}

// matchHistoryResponse is the 200 body for GET /v1/matches/history.
type matchHistoryResponse struct {
	Matches []matchHistoryItemResponse `json:"matches"`
}

// defaultPageLimit is the default number of items per page.
const defaultPageLimit = 20

// maxPageLimit is the maximum allowed page size (cap).
const maxPageLimit = 50

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
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
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

// GetMatchHistory handles GET /v1/matches/history.
func (h *ratingsHandler) GetMatchHistory(c echo.Context) error {
	// user_id — required, positive integer.
	rawUserID := c.QueryParam("user_id")
	if rawUserID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "user_id is required")
	}

	userID, err := strconv.ParseInt(rawUserID, 10, 64)
	if err != nil || userID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "user_id must be a positive integer")
	}

	// limit — default 20, capped at 50, non-negative.
	limit := defaultPageLimit

	if rawLimit := c.QueryParam("limit"); rawLimit != "" {
		l, parseErr := strconv.Atoi(rawLimit)
		if parseErr != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "limit must be a non-negative integer")
		}

		if l < 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "limit must be a non-negative integer")
		}

		if l > maxPageLimit {
			l = maxPageLimit
		}

		limit = l
	}

	// offset — default 0, non-negative.
	offset := 0

	if rawOffset := c.QueryParam("offset"); rawOffset != "" {
		o, parseErr := strconv.Atoi(rawOffset)
		if parseErr != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "offset must be a non-negative integer")
		}

		if o < 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "offset must be a non-negative integer")
		}

		offset = o
	}

	items, err := h.service.ListMatchHistory(c.Request().Context(), userID, limit, offset)
	if err != nil {
		h.logger.Error("list match history", zap.Int64("user_id", userID), zap.Error(err))

		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	resp := matchHistoryResponse{
		Matches: make([]matchHistoryItemResponse, 0, len(items)),
	}

	for _, item := range items {
		resp.Matches = append(resp.Matches, matchHistoryItemResponse{
			MatchID:    item.MatchID,
			OpponentID: item.OpponentID,
			Result:     item.Result,
			Mode:       item.Mode,
			ELODelta:   item.ELODelta,
			DurationMS: item.DurationMS,
			CreatedAt:  item.CreatedAt,
		})
	}

	return c.JSON(http.StatusOK, resp)
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
