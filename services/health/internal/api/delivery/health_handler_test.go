package delivery

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/pizdagladki/full/services/health/internal/api/domain"
	"github.com/pizdagladki/full/services/health/internal/api/service"
	"github.com/pizdagladki/full/services/health/internal/api/service/mocks"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

func TestHealthHandler_Get(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		newService func(t *testing.T) service.HealthService
		wantStatus string
	}{
		{
			name: "real service returns ok",
			newService: func(_ *testing.T) service.HealthService {
				return service.NewHealthService()
			},
			wantStatus: "ok",
		},
		{
			name: "mocked service status is served",
			newService: func(t *testing.T) service.HealthService {
				ctrl := gomock.NewController(t)
				m := mocks.NewMockHealthService(ctrl)
				m.EXPECT().Check().Return(domain.HealthStatus{Status: "degraded"})

				return m
			},
			wantStatus: "degraded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			h := NewHealthHandler(tt.newService(t), zap.NewNop())

			if err := h.Get(c); err != nil {
				t.Fatalf("Get() error = %v", err)
			}

			if rec.Code != http.StatusOK {
				t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
			}
			if ct := rec.Header().Get(echo.HeaderContentType); ct != echo.MIMEApplicationJSONCharsetUTF8 {
				t.Errorf("Content-Type = %q, want %q", ct, echo.MIMEApplicationJSONCharsetUTF8)
			}

			var got domain.HealthStatus
			if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if got.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", got.Status, tt.wantStatus)
			}
		})
	}
}

// failingResponseWriter fails on Write to exercise the encode-error branch.
type failingResponseWriter struct {
	header http.Header
}

func (f *failingResponseWriter) Header() http.Header {
	if f.header == nil {
		f.header = make(http.Header)
	}

	return f.header
}

func (f *failingResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func (f *failingResponseWriter) WriteHeader(int) {}

func TestHealthHandler_Get_EncodeError(t *testing.T) {
	t.Parallel()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	c := e.NewContext(req, &failingResponseWriter{})

	h := NewHealthHandler(service.NewHealthService(), zap.NewNop())

	if err := h.Get(c); err == nil {
		t.Fatal("expected an error when the response write fails, got nil")
	}
}
