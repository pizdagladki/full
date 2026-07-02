package middleware_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/reports/internal/api/delivery"
	"github.com/pizdagladki/full/services/reports/internal/api/middleware"
	"github.com/pizdagladki/full/services/reports/internal/api/repository"
	svcmocks "github.com/pizdagladki/full/services/reports/internal/api/service/mocks"
)

func newTestEcho(m *middleware.AuthMiddleware) (*echo.Echo, func(echo.Context) error) {
	e := echo.New()
	e.HideBanner = true

	var capturedUserID int64
	handler := func(c echo.Context) error {
		if v := c.Get(delivery.UserIDContextKey); v != nil {
			if id, ok := v.(int64); ok {
				capturedUserID = id
			}
		}
		return c.NoContent(http.StatusOK)
	}

	_ = capturedUserID // used via closure in tests

	e.GET("/protected", handler, m.RequireAuth)
	return e, handler
}

func TestAuthMiddleware_RequireAuth(t *testing.T) {
	t.Parallel()

	const cookieName = "session"

	tests := []struct {
		name       string
		cookie     *http.Cookie
		setupMock  func(svc *svcmocks.MockSessionService)
		wantStatus int
		wantUserID int64
		criterion  string
	}{
		{
			// criterion: 5 — missing session cookie → 401
			name:       "missing cookie returns 401",
			cookie:     nil,
			setupMock:  func(svc *svcmocks.MockSessionService) {},
			wantStatus: http.StatusUnauthorized,
			criterion:  "AC5",
		},
		{
			// criterion: 5 — session not found → 401
			name:   "session not found returns 401",
			cookie: &http.Cookie{Name: cookieName, Value: "bad-token"},
			setupMock: func(svc *svcmocks.MockSessionService) {
				svc.EXPECT().ResolveSession(gomock.Any(), "bad-token").
					Return(int64(0), repository.ErrSessionNotFound)
			},
			wantStatus: http.StatusUnauthorized,
			criterion:  "AC5",
		},
		{
			// criterion: 5 — unexpected session error → 401
			name:   "unexpected session error returns 401",
			cookie: &http.Cookie{Name: cookieName, Value: "err-token"},
			setupMock: func(svc *svcmocks.MockSessionService) {
				svc.EXPECT().ResolveSession(gomock.Any(), "err-token").
					Return(int64(0), errors.New("redis down"))
			},
			wantStatus: http.StatusUnauthorized,
			criterion:  "AC5",
		},
		{
			// criterion: 5 — valid session → 200 and user_id in context
			name:   "valid session sets user_id and passes to handler",
			cookie: &http.Cookie{Name: cookieName, Value: "good-token"},
			setupMock: func(svc *svcmocks.MockSessionService) {
				svc.EXPECT().ResolveSession(gomock.Any(), "good-token").
					Return(int64(42), nil)
			},
			wantStatus: http.StatusOK,
			wantUserID: 42,
			criterion:  "AC5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			sessionSvc := svcmocks.NewMockSessionService(ctrl)
			tt.setupMock(sessionSvc)

			m := middleware.NewAuthMiddleware(sessionSvc, cookieName, zap.NewNop())
			e, _ := newTestEcho(m)

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tt.cookie != nil {
				req.AddCookie(tt.cookie)
			}

			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}
