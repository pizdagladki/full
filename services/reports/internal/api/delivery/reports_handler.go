package delivery

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/reports/internal/api/service"
)

const devicePC = "pc"

type reportsHandler struct {
	svc            service.ReportsService
	bugSvc         service.BugReportService
	maxUploadBytes int64
	logger         *zap.Logger
}

// NewReportsHandler constructs an HTTP handler for cheat-report and bug-report
// endpoints.
func NewReportsHandler(
	svc service.ReportsService,
	bugSvc service.BugReportService,
	maxUploadBytes int64,
	logger *zap.Logger,
) ReportsHandler {
	return &reportsHandler{
		svc:            svc,
		bugSvc:         bugSvc,
		maxUploadBytes: maxUploadBytes,
		logger:         logger,
	}
}

// postCheatReportRequest is the request body for POST /v1/reports/cheat.
// ReporterID is optional; if absent (zero) the self-report check is skipped.
// The documented minimal body is {reported_id, match_id}.
type postCheatReportRequest struct {
	ReporterID int64  `json:"reporter_id"` // optional; future: from auth session
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

// PostBugReport handles POST /v1/reports/bug (multipart form).
// Returns 201 on success, 400 on validation failures, 401 when unauthenticated.
func (h *reportsHandler) PostBugReport(c echo.Context) error {
	// Extract authenticated user ID injected by the auth middleware.
	val := c.Get(UserIDContextKey)
	if val == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}

	userID, ok := val.(int64)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid user id in context")
	}

	device := c.FormValue("device")
	description := c.FormValue("description")

	if device != "mobile" && device != devicePC {
		return echo.NewHTTPError(http.StatusBadRequest, "device must be mobile or pc")
	}

	var recordingBytes []byte

	if device == devicePC {
		file, header, err := c.Request().FormFile("recording")
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "recording required for pc device")
		}
		defer file.Close()

		if ct := header.Header.Get("Content-Type"); ct != "video/webm" {
			return echo.NewHTTPError(http.StatusBadRequest, "recording must be video/webm")
		}

		limited := io.LimitReader(file, h.maxUploadBytes+1)

		data, err := io.ReadAll(limited)
		if err != nil {
			h.logger.Error("read recording failed", zap.Error(err))

			return echo.NewHTTPError(http.StatusInternalServerError, "read recording failed")
		}

		if int64(len(data)) > h.maxUploadBytes {
			return echo.NewHTTPError(http.StatusBadRequest, "recording too large")
		}

		recordingBytes = data
	}

	err := h.bugSvc.ReportBug(c.Request().Context(), userID, device, description, recordingBytes)
	if err != nil {
		h.logger.Error("report bug failed", zap.Error(err))

		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	return c.NoContent(http.StatusCreated)
}
