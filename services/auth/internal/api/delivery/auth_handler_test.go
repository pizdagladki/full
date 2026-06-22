package delivery_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/auth/internal/api/delivery"
	"github.com/pizdagladki/full/services/auth/internal/api/domain"
	"github.com/pizdagladki/full/services/auth/internal/api/service"
	svcmocks "github.com/pizdagladki/full/services/auth/internal/api/service/mocks"
)

// echoValidator adapts validator/v10 for echo in tests.
type echoValidator struct{ v *validator.Validate }

func (ev *echoValidator) Validate(i any) error { return ev.v.Struct(i) }

func newTestEcho() *echo.Echo {
	e := echo.New()
	e.Validator = &echoValidator{v: validator.New()}

	return e
}

func TestAuthHandler_LoginGoogle(t *testing.T) {
	t.Parallel()

	fixedUser := domain.User{ID: 1, Email: "alice@example.com"}

	tests := []struct {
		name          string
		body          string
		setupSvc      func(m *svcmocks.MockAuthService)
		wantStatus    int
		wantCookieSet bool
		wantBodyID    *int64
	}{
		{
			name: "valid code - 200 + HttpOnly cookie set",
			body: `{"code":"valid-code"}`,
			setupSvc: func(m *svcmocks.MockAuthService) {
				m.EXPECT().LoginGoogle(gomock.Any(), "valid-code").
					Return("sess-abc", fixedUser, nil)
			},
			wantStatus:    http.StatusOK,
			wantCookieSet: true,
			wantBodyID:    func() *int64 { v := int64(1); return &v }(),
		},
		{
			name:          "malformed JSON - 400, no cookie",
			body:          `{not-json`,
			setupSvc:      func(_ *svcmocks.MockAuthService) {},
			wantStatus:    http.StatusBadRequest,
			wantCookieSet: false,
		},
		{
			name:          "missing code field - 400, no cookie",
			body:          `{}`,
			setupSvc:      func(_ *svcmocks.MockAuthService) {},
			wantStatus:    http.StatusBadRequest,
			wantCookieSet: false,
		},
		{
			name: "invalid code - 401, no cookie",
			body: `{"code":"bad-code"}`,
			setupSvc: func(m *svcmocks.MockAuthService) {
				m.EXPECT().LoginGoogle(gomock.Any(), "bad-code").
					Return("", domain.User{}, service.ErrInvalidCode)
			},
			wantStatus:    http.StatusUnauthorized,
			wantCookieSet: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			svcMock := svcmocks.NewMockAuthService(ctrl)
			tt.setupSvc(svcMock)

			h := delivery.NewAuthHandler(svcMock, zap.NewNop(), delivery.HandlerConfig{
				CookieName:   "session",
				CookieTTL:    24 * time.Hour,
				CookieSecure: false,
			})

			e := newTestEcho()
			req := httptest.NewRequest(http.MethodPost, "/v1/auth/google", strings.NewReader(tt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			_ = h.LoginGoogle(c)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			cookies := rec.Result().Cookies()
			hasCookie := false
			for _, ck := range cookies {
				if ck.Name == "session" {
					hasCookie = true
					if !ck.HttpOnly {
						t.Error("session cookie is not HttpOnly")
					}
				}
			}
			if hasCookie != tt.wantCookieSet {
				t.Errorf("cookie set = %v, want %v", hasCookie, tt.wantCookieSet)
			}

			if tt.wantBodyID != nil {
				var resp domain.MeResponse
				if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
					t.Fatalf("decode response body: %v", err)
				}
				if resp.ID != *tt.wantBodyID {
					t.Errorf("body.id = %d, want %d", resp.ID, *tt.wantBodyID)
				}
			}
		})
	}
}

func TestAuthHandler_GetMe(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		ctxUser    any // what to set under UserContextKey
		wantStatus int
		wantEmail  string
	}{
		{
			name:       "valid user in context - 200",
			ctxUser:    domain.User{ID: 7, Email: "bob@example.com"},
			wantStatus: http.StatusOK,
			wantEmail:  "bob@example.com",
		},
		{
			name:       "no user in context - 401",
			ctxUser:    nil,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "wrong type in context - 401",
			ctxUser:    "not-a-user",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			svcMock := svcmocks.NewMockAuthService(ctrl)

			h := delivery.NewAuthHandler(svcMock, zap.NewNop(), delivery.HandlerConfig{
				CookieName: "session",
				CookieTTL:  24 * time.Hour,
			})

			e := newTestEcho()
			req := httptest.NewRequest(http.MethodGet, "/v1/auth/me", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			if tt.ctxUser != nil {
				c.Set(delivery.UserContextKey, tt.ctxUser)
			}

			_ = h.GetMe(c)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			if tt.wantEmail != "" {
				var resp domain.MeResponse
				if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				if resp.Email != tt.wantEmail {
					t.Errorf("email = %q, want %q", resp.Email, tt.wantEmail)
				}
			}
		})
	}
}
