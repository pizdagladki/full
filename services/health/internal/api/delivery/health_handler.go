package delivery

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pizdagladki/full/services/health/internal/api/service"
	"go.uber.org/zap"
)

type healthHandler struct {
	service service.HealthService
	logger  *zap.Logger
}

// NewHealthHandler builds a HealthHandler.
func NewHealthHandler(svc service.HealthService, logger *zap.Logger) HealthHandler {
	return &healthHandler{service: svc, logger: logger}
}

// Get handles GET /v1/health.
func (h *healthHandler) Get(c echo.Context) error {
	status := h.service.Check()

	err := c.JSON(http.StatusOK, status)
	if err != nil {
		h.logger.Error("encode health response", zap.Error(err))
	}

	return err
}
