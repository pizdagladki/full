package middleware_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/auth/internal/api/delivery"
	"github.com/pizdagladki/full/services/auth/internal/api/domain"
	"github.com/pizdagladki/full/services/auth/internal/api/middleware"
	"github.com/pizdagladki/full/services/auth/internal/api/service"
	svcmocks "github.com/pizdagladki/full/services/auth/internal/api/service/mocks"
)

const testCookieName = "session"

func TestRequireAuth(t *testing.T) {
	t.Parallel()

	fixedUser := domain.User{ID: 3, Email: "dave@example.com"}

	tests := []struct {
		name       string
		cookie     *http.Cookie // nil means no cookie
		setupSvc   func(m *svcmocks.MockAuthService)
		wantStatus int
		wantUser   *domain.User // non-nil means middleware passed to handler
	}{
		{
			name:       "no cookie - 401",
			cookie:     nil,
			setupSvc:   func(_ *svcmocks.MockAuthService) {},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:   "invalid/expired session - 401",
			cookie: &http.Cookie{Name: testCookieName, Value: "stale-id"},
			setupSvc: func(m *svcmocks.MockAuthService) {
				m.EXPECT().Authenticate(gomock.Any(), "stale-id").
					Return(domain.User{}, service.ErrSessionNotFound)
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:   "unknown authenticate error - 401",
			cookie: &http.Cookie{Name: testCookieName, Value: "bad-id"},
			setupSvc: func(m *svcmocks.MockAuthService) {
				m.EXPECT().Authenticate(gomock.Any(), "bad-id").
					Return(domain.User{}, errors.New("db timeout"))
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:   "valid session - 200 and user in context",
			cookie: &http.Cookie{Name: testCookieName, Value: "valid-id"},
			setupSvc: func(m *svcmocks.MockAuthService) {
				m.EXPECT().Authenticate(gomock.Any(), "valid-id").
					Return(fixedUser, nil)
			},
			wantStatus: http.StatusOK,
			wantUser:   &fixedUser,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			svcMock := svcmocks.NewMockAuthService(ctrl)
			tt.setupSvc(svcMock)

			mw := middleware.NewAuthMiddleware(svcMock, testCookieName, zap.NewNop())

			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/v1/auth/me", nil)
			if tt.cookie != nil {
				req.AddCookie(tt.cookie)
			}
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			var capturedUser *domain.User

			next := func(c echo.Context) error {
				if u, ok := c.Get(delivery.UserContextKey).(domain.User); ok {
					capturedUser = &u
				}

				return c.NoContent(http.StatusOK)
			}

			handler := mw.RequireAuth(next)
			_ = handler(c)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			if tt.wantUser != nil {
				if capturedUser == nil {
					t.Fatal("user not set in context, want user")
				}
				if capturedUser.ID != tt.wantUser.ID {
					t.Errorf("context user.ID = %d, want %d", capturedUser.ID, tt.wantUser.ID)
				}
			} else if capturedUser != nil {
				t.Errorf("user set in context = %+v, want nil on error path", capturedUser)
			}
		})
	}
}
