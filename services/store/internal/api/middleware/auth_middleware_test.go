package middleware_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/store/internal/api/delivery"
	"github.com/pizdagladki/full/services/store/internal/api/middleware"
	"github.com/pizdagladki/full/services/store/internal/api/repository"
	svcmocks "github.com/pizdagladki/full/services/store/internal/api/service/mocks"
)

const testCookieName = "session"

func TestRequireAuth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		cookie     *http.Cookie
		setupSvc   func(m *svcmocks.MockSessionService)
		wantStatus int
		wantUserID *int64
	}{
		{
			name:       "no cookie returns 401",
			cookie:     nil,
			setupSvc:   func(_ *svcmocks.MockSessionService) {},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:   "expired/missing session returns 401",
			cookie: &http.Cookie{Name: testCookieName, Value: "stale-id"},
			setupSvc: func(m *svcmocks.MockSessionService) {
				m.EXPECT().ResolveSession(gomock.Any(), "stale-id").
					Return(int64(0), repository.ErrSessionNotFound)
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:   "unexpected resolve error returns 401",
			cookie: &http.Cookie{Name: testCookieName, Value: "broken"},
			setupSvc: func(m *svcmocks.MockSessionService) {
				m.EXPECT().ResolveSession(gomock.Any(), "broken").
					Return(int64(0), errors.New("redis timeout"))
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:   "valid session sets user id in context and calls next",
			cookie: &http.Cookie{Name: testCookieName, Value: "valid-id"},
			setupSvc: func(m *svcmocks.MockSessionService) {
				m.EXPECT().ResolveSession(gomock.Any(), "valid-id").
					Return(int64(42), nil)
			},
			wantStatus: http.StatusOK,
			wantUserID: func() *int64 { v := int64(42); return &v }(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			svcMock := svcmocks.NewMockSessionService(ctrl)
			tt.setupSvc(svcMock)

			mw := middleware.NewAuthMiddleware(svcMock, testCookieName, zap.NewNop())

			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/v1/store/inventory", nil)
			if tt.cookie != nil {
				req.AddCookie(tt.cookie)
			}
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			var capturedUserID *int64

			next := func(c echo.Context) error {
				if uid, ok := c.Get(delivery.UserIDContextKey).(int64); ok {
					capturedUserID = &uid
				}

				return c.NoContent(http.StatusOK)
			}

			handler := mw.RequireAuth(next)
			_ = handler(c)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			if tt.wantUserID != nil {
				if capturedUserID == nil {
					t.Fatal("user id not set in context, want user id")
				}

				if *capturedUserID != *tt.wantUserID {
					t.Errorf("context user id = %d, want %d", *capturedUserID, *tt.wantUserID)
				}
			} else if capturedUserID != nil {
				t.Errorf("user id set in context = %d, want nil on error path", *capturedUserID)
			}
		})
	}
}
