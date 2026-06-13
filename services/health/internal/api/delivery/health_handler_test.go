package delivery

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pizdagladki/full/services/health/internal/api/domain"
	"github.com/pizdagladki/full/services/health/internal/api/service"
	"go.uber.org/zap"
)

func TestHealthHandler_Get(t *testing.T) {
	h := NewHealthHandler(service.NewHealthService(), zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	rec := httptest.NewRecorder()

	h.Get(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var got domain.HealthStatus
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got.Status != "ok" {
		t.Errorf("status = %q, want %q", got.Status, "ok")
	}
}
