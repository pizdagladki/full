package delivery

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/reports/internal/api/service"
)

type reportsHandler struct {
	svc    service.ReportsService
	logger *zap.Logger
}

// NewReportsHandler constructs an HTTP handler for cheat-report endpoints.
func NewReportsHandler(svc service.ReportsService, logger *zap.Logger) ReportsHandler {
	return &reportsHandler{svc: svc, logger: logger}
}

// postCheatReportRequest is the request body for POST /v1/reports/cheat.
type postCheatReportRequest struct {
	ReporterID int64  `json:"reporter_id" validate:"required"`
	ReportedID int64  `json:"reported_id" validate:"required"`
	MatchID    string `json:"match_id"    validate:"required"`
}

// cooldownResponse is the response body for GET /v1/reports/cooldown/:user_id.
type cooldownResponse struct {
	Active           bool `json:"active"`
	SecondsRemaining int  `json:"seconds_remaining"`
}

// PostCheatReport handles POST /v1/reports/cheat.
// Returns 201 on success, 400 on validation failure or self-report.
func (h *reportsHandler) PostCheatReport(c echo.Context) error {
	var req postCheatReportRequest

	err := c.Bind(&req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	err = c.Validate(&req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	err = h.svc.ReportCheat(c.Request().Context(), req.ReporterID, req.ReportedID, req.MatchID)
	if err != nil {
		if errors.Is(err, service.ErrSelfReport) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}

		h.logger.Error("report cheat failed", zap.Error(err))

		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	return c.NoContent(http.StatusCreated)
}

// GetCooldown handles GET /v1/reports/cooldown/:user_id.
// Returns 200 with {active, seconds_remaining}.
func (h *reportsHandler) GetCooldown(c echo.Context) error {
	userIDStr := c.Param("user_id")

	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user_id")
	}

	status, err := h.svc.GetCooldown(c.Request().Context(), userID)
	if err != nil {
		h.logger.Error("get cooldown failed", zap.Int64("user_id", userID), zap.Error(err))

		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	return c.JSON(http.StatusOK, cooldownResponse{
		Active:           status.Active,
		SecondsRemaining: status.SecondsRemaining,
	})
}
