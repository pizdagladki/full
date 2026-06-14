package delivery

import (
	"encoding/json"
	"net/http"

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
func (h *healthHandler) Get(w http.ResponseWriter, _ *http.Request) {
	status := h.service.Check()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	err := json.NewEncoder(w).Encode(status)
	if err != nil {
		h.logger.Error("encode health response", zap.Error(err))
	}
}
