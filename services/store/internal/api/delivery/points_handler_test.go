package delivery_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/internal/platform/internalauth"
	"github.com/pizdagladki/full/services/store/internal/api/delivery"
	"github.com/pizdagladki/full/services/store/internal/api/domain"
	svcmocks "github.com/pizdagladki/full/services/store/internal/api/service/mocks"
)

// setupEchoWithValidator builds an *echo.Echo wired with the same
// validator/v10 Validator as newEcho() (see purchase_handler_test.go), reused
// here so bound request DTOs are actually validated.
func setupEchoWithValidator(t *testing.T) *echo.Echo {
	t.Helper()

	e := newEcho()
	e.HideBanner = true

	return e
}

func TestPointsHandler_Credit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		body            string
		setupSvc        func(m *svcmocks.MockPointsService)
		wantStatus      int
		wantBalance     int64
		wantBodyExclude string
	}{
		{
			// criterion: 1 — a valid credit request returns 200 with the new balance.
			name: "valid credit returns 200 with new balance",
			body: `{"user_id":1,"reason":"match_win","ref_id":"m-1"}`,
			setupSvc: func(m *svcmocks.MockPointsService) {
				m.EXPECT().Credit(gomock.Any(), domain.PointsCredit{
					UserID: 1, Reason: "match_win", RefID: "m-1", Delta: 0,
				}).Return(int64(10), nil)
			},
			wantStatus:  http.StatusOK,
			wantBalance: 10,
		},
		{
			// criterion: 2 — a duplicate/idempotent credit still returns 200 with the
			// (unchanged) existing balance, not an error.
			name: "idempotent duplicate returns 200 with existing balance",
			body: `{"user_id":1,"reason":"match_win","ref_id":"m-dup"}`,
			setupSvc: func(m *svcmocks.MockPointsService) {
				m.EXPECT().Credit(gomock.Any(), gomock.Any()).Return(int64(50), nil)
			},
			wantStatus:  http.StatusOK,
			wantBalance: 50,
		},
		{
			// criterion: 1 — an explicit positive delta is forwarded to the service.
			name: "explicit delta forwarded to service",
			body: `{"user_id":2,"reason":"custom","delta":100}`,
			setupSvc: func(m *svcmocks.MockPointsService) {
				m.EXPECT().Credit(gomock.Any(), domain.PointsCredit{
					UserID: 2, Reason: "custom", RefID: "", Delta: 100,
				}).Return(int64(100), nil)
			},
			wantStatus:  http.StatusOK,
			wantBalance: 100,
		},
		{
			// criterion: 4 — an empty reason / non-positive resolved delta maps
			// ErrInvalidCredit to 400.
			name: "invalid credit from service returns 400",
			body: `{"user_id":1,"reason":"","ref_id":"x"}`,
			setupSvc: func(m *svcmocks.MockPointsService) {
				m.EXPECT().Credit(gomock.Any(), gomock.Any()).Return(int64(0), domain.ErrInvalidCredit)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 4 — malformed/missing user_id fails request validation
			// before even reaching the service (400).
			name:       "missing user_id fails validation returns 400",
			body:       `{"reason":"match_win"}`,
			setupSvc:   func(_ *svcmocks.MockPointsService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "malformed json body returns 400",
			body:       `{"user_id":`,
			setupSvc:   func(_ *svcmocks.MockPointsService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "service internal error returns 500 with generic body",
			body: `{"user_id":1,"reason":"match_win","ref_id":"x"}`,
			setupSvc: func(m *svcmocks.MockPointsService) {
				m.EXPECT().Credit(gomock.Any(), gomock.Any()).Return(int64(0), errors.New("db down"))
			},
			wantStatus:      http.StatusInternalServerError,
			wantBodyExclude: "db down",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			svc := svcmocks.NewMockPointsService(ctrl)
			tt.setupSvc(svc)

			h := delivery.NewPointsHandler(svc, zap.NewNop())

			e := setupEchoWithValidator(t)
			req := httptest.NewRequest(http.MethodPost, "/v1/points/credit", strings.NewReader(tt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			_ = h.Credit(c)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d, body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			body := rec.Body.String()

			if tt.wantBodyExclude != "" && strings.Contains(body, tt.wantBodyExclude) {
				t.Errorf("body must not contain internal detail %q but got: %s", tt.wantBodyExclude, body)
			}

			if tt.wantStatus == http.StatusOK {
				var resp map[string]any
				if err := json.Unmarshal([]byte(body), &resp); err != nil {
					t.Fatalf("decode response: %v", err)
				}

				got, _ := resp["balance"].(float64)
				if int64(got) != tt.wantBalance {
					t.Errorf("balance = %v, want %d", got, tt.wantBalance)
				}
			}
		})
	}
}

// TestPointsHandler_Credit_ThroughInternalAuth exercises POST
// /v1/points/credit wrapped with the internalauth middleware, the way it is
// actually registered in register_http_routes.go, so the route-level
// criteria (no header -> 401, wrong token -> 401, correct token -> existing
// 2xx behavior) are verified end to end rather than just at the handler.
func TestPointsHandler_Credit_ThroughInternalAuth(t *testing.T) {
	t.Parallel()

	const configuredToken = "s2s-secret-token"

	tests := []struct {
		name          string
		setHeader     bool
		authHeader    string
		setupSvc      func(m *svcmocks.MockPointsService)
		wantStatus    int
		wantSvcCalled bool
	}{
		{
			// criterion: no header -> 401, business logic never runs.
			name:          "no Authorization header -> 401",
			setHeader:     false,
			setupSvc:      func(_ *svcmocks.MockPointsService) {},
			wantStatus:    http.StatusUnauthorized,
			wantSvcCalled: false,
		},
		{
			// criterion: wrong token -> 401, business logic never runs.
			name:          "wrong token -> 401",
			setHeader:     true,
			authHeader:    "Bearer not-the-token",
			setupSvc:      func(_ *svcmocks.MockPointsService) {},
			wantStatus:    http.StatusUnauthorized,
			wantSvcCalled: false,
		},
		{
			// criterion: correct token -> existing 2xx behavior preserved.
			name:       "correct token -> existing 200 behavior",
			setHeader:  true,
			authHeader: "Bearer " + configuredToken,
			setupSvc: func(m *svcmocks.MockPointsService) {
				m.EXPECT().Credit(gomock.Any(), domain.PointsCredit{
					UserID: 1, Reason: "match_win", RefID: "m-1", Delta: 0,
				}).Return(int64(10), nil)
			},
			wantStatus:    http.StatusOK,
			wantSvcCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			svc := svcmocks.NewMockPointsService(ctrl)
			tt.setupSvc(svc)

			handler := delivery.NewPointsHandler(svc, zap.NewNop())
			guarded := internalauth.New(configuredToken)(handler.Credit)

			e := setupEchoWithValidator(t)
			req := httptest.NewRequest(http.MethodPost, "/v1/points/credit", strings.NewReader(`{"user_id":1,"reason":"match_win","ref_id":"m-1"}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			if tt.setHeader {
				req.Header.Set(echo.HeaderAuthorization, tt.authHeader)
			}
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			if err := guarded(c); err != nil {
				t.Fatalf("guarded handler returned error: %v", err)
			}

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d, body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestPointsHandler_GetBalance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		userID          any
		setupSvc        func(m *svcmocks.MockPointsService)
		wantStatus      int
		wantBalance     int64
		wantBodyExclude string
	}{
		{
			// criterion: 3 — an authenticated user gets 200 with the balance.
			name:   "authenticated user returns balance",
			userID: int64(42),
			setupSvc: func(m *svcmocks.MockPointsService) {
				m.EXPECT().GetBalance(gomock.Any(), int64(42)).Return(int64(77), nil)
			},
			wantStatus:  http.StatusOK,
			wantBalance: 77,
		},
		{
			// criterion: 3 — no session user in context (RequireAuth would have 401'd
			// upstream, but the handler itself also enforces this) returns 401.
			name:       "no user id in context returns 401",
			userID:     nil,
			setupSvc:   func(_ *svcmocks.MockPointsService) {},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:   "service error returns 500 with generic body",
			userID: int64(1),
			setupSvc: func(m *svcmocks.MockPointsService) {
				m.EXPECT().GetBalance(gomock.Any(), int64(1)).Return(int64(0), errors.New("db down"))
			},
			wantStatus:      http.StatusInternalServerError,
			wantBodyExclude: "db down",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			svc := svcmocks.NewMockPointsService(ctrl)
			tt.setupSvc(svc)

			h := delivery.NewPointsHandler(svc, zap.NewNop())

			e := setupEchoWithValidator(t)
			req := httptest.NewRequest(http.MethodGet, "/v1/points/balance", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			if tt.userID != nil {
				c.Set(delivery.UserIDContextKey, tt.userID)
			}

			_ = h.GetBalance(c)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			body := rec.Body.String()

			if tt.wantBodyExclude != "" && strings.Contains(body, tt.wantBodyExclude) {
				t.Errorf("body must not contain internal detail %q but got: %s", tt.wantBodyExclude, body)
			}

			if tt.wantStatus == http.StatusOK {
				var resp map[string]any
				if err := json.Unmarshal([]byte(body), &resp); err != nil {
					t.Fatalf("decode response: %v", err)
				}

				got, _ := resp["balance"].(float64)
				if int64(got) != tt.wantBalance {
					t.Errorf("balance = %v, want %d", got, tt.wantBalance)
				}
			}
		})
	}
}
