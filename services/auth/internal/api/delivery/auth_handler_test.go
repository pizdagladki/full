package delivery_test

import (
	"encoding/json"
	"errors"
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

// newHandler is a helper that creates an AuthHandler with stub mocks for both
// AuthService and ConsentService so tests that only care about one can ignore
// the other.
func newHandler(t *testing.T, authSvc service.AuthService, consentSvc service.ConsentService) delivery.AuthHandler {
	t.Helper()

	return delivery.NewAuthHandler(authSvc, consentSvc, zap.NewNop(), delivery.HandlerConfig{
		CookieName:   "session",
		CookieTTL:    24 * time.Hour,
		CookieSecure: false,
	})
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
			authMock := svcmocks.NewMockAuthService(ctrl)
			consentMock := svcmocks.NewMockConsentService(ctrl)
			tt.setupSvc(authMock)

			h := newHandler(t, authMock, consentMock)

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

	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	fixedConsent := &domain.Consent{
		IsAdult:          true,
		ConsentRecording: true,
		ConsentTos:       true,
		AcceptedAt:       fixedTime,
	}

	tests := []struct {
		name            string
		ctxUser         any // what to set under UserContextKey
		setupConsentSvc func(m *svcmocks.MockConsentService)
		wantStatus      int
		wantEmail       string
		wantConsent     bool // whether resp.Consent should be non-null
	}{
		{
			name:    "valid user in context with consent - 200",
			ctxUser: domain.User{ID: 7, Email: "bob@example.com"},
			setupConsentSvc: func(m *svcmocks.MockConsentService) {
				m.EXPECT().GetConsent(gomock.Any(), int64(7)).Return(fixedConsent, nil)
			},
			wantStatus:  http.StatusOK,
			wantEmail:   "bob@example.com",
			wantConsent: true,
		},
		{
			name:    "valid user in context with no consent yet - consent null",
			ctxUser: domain.User{ID: 8, Email: "carol@example.com"},
			setupConsentSvc: func(m *svcmocks.MockConsentService) {
				m.EXPECT().GetConsent(gomock.Any(), int64(8)).Return(nil, nil)
			},
			wantStatus:  http.StatusOK,
			wantEmail:   "carol@example.com",
			wantConsent: false,
		},
		{
			name:            "no user in context - 401",
			ctxUser:         nil,
			setupConsentSvc: func(_ *svcmocks.MockConsentService) {},
			wantStatus:      http.StatusUnauthorized,
		},
		{
			name:            "wrong type in context - 401",
			ctxUser:         "not-a-user",
			setupConsentSvc: func(_ *svcmocks.MockConsentService) {},
			wantStatus:      http.StatusUnauthorized,
		},
		{
			name:    "consent service error - 500",
			ctxUser: domain.User{ID: 9, Email: "dave@example.com"},
			setupConsentSvc: func(m *svcmocks.MockConsentService) {
				m.EXPECT().GetConsent(gomock.Any(), int64(9)).Return(nil, errors.New("db error"))
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			authMock := svcmocks.NewMockAuthService(ctrl)
			consentMock := svcmocks.NewMockConsentService(ctrl)
			tt.setupConsentSvc(consentMock)

			h := newHandler(t, authMock, consentMock)

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
				if tt.wantConsent && resp.Consent == nil {
					t.Error("resp.Consent = nil, want non-null consent object")
				}
				if !tt.wantConsent && resp.Consent != nil {
					t.Errorf("resp.Consent = %+v, want null", resp.Consent)
				}
			}
		})
	}
}

// TestAuthHandler_SubmitConsent covers acceptance criteria 1, 2, 3, 4.
func TestAuthHandler_SubmitConsent(t *testing.T) {
	t.Parallel()

	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	allTrue := domain.ConsentRequest{IsAdult: true, ConsentRecording: true, ConsentTos: true}
	returnedConsent := domain.Consent{
		IsAdult:          true,
		ConsentRecording: true,
		ConsentTos:       true,
		AcceptedAt:       fixedTime,
	}

	tests := []struct {
		name            string
		ctxUser         any
		body            string
		setupConsentSvc func(m *svcmocks.MockConsentService)
		wantStatus      int
	}{
		{
			// Criterion 1: all three flags true → 200, service called once.
			name:    "all flags true - 200",
			ctxUser: domain.User{ID: 42, Email: "alice@example.com"},
			body:    `{"is_adult":true,"consent_recording":true,"consent_tos":true}`,
			setupConsentSvc: func(m *svcmocks.MockConsentService) {
				m.EXPECT().RecordConsent(gomock.Any(), int64(42), allTrue).
					Return(returnedConsent, nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			// Criterion 2: is_adult explicitly false → 422, service NOT called.
			name:            "is_adult false - 422",
			ctxUser:         domain.User{ID: 42, Email: "alice@example.com"},
			body:            `{"is_adult":false,"consent_recording":true,"consent_tos":true}`,
			setupConsentSvc: func(_ *svcmocks.MockConsentService) {},
			wantStatus:      http.StatusUnprocessableEntity,
		},
		{
			// Criterion 2: consent_recording explicitly false → 422.
			name:            "consent_recording false - 422",
			ctxUser:         domain.User{ID: 42, Email: "alice@example.com"},
			body:            `{"is_adult":true,"consent_recording":false,"consent_tos":true}`,
			setupConsentSvc: func(_ *svcmocks.MockConsentService) {},
			wantStatus:      http.StatusUnprocessableEntity,
		},
		{
			// Criterion 2: consent_tos explicitly false → 422.
			name:            "consent_tos false - 422",
			ctxUser:         domain.User{ID: 42, Email: "alice@example.com"},
			body:            `{"is_adult":true,"consent_recording":true,"consent_tos":false}`,
			setupConsentSvc: func(_ *svcmocks.MockConsentService) {},
			wantStatus:      http.StatusUnprocessableEntity,
		},
		{
			// Criterion 2: missing fields → 422.
			name:            "missing fields - 422",
			ctxUser:         domain.User{ID: 42, Email: "alice@example.com"},
			body:            `{}`,
			setupConsentSvc: func(_ *svcmocks.MockConsentService) {},
			wantStatus:      http.StatusUnprocessableEntity,
		},
		{
			// Criterion 2: only one field set → 422.
			name:            "only is_adult set - 422",
			ctxUser:         domain.User{ID: 42, Email: "alice@example.com"},
			body:            `{"is_adult":true}`,
			setupConsentSvc: func(_ *svcmocks.MockConsentService) {},
			wantStatus:      http.StatusUnprocessableEntity,
		},
		{
			// Malformed JSON → 400.
			name:            "malformed JSON - 400",
			ctxUser:         domain.User{ID: 42, Email: "alice@example.com"},
			body:            `{bad json`,
			setupConsentSvc: func(_ *svcmocks.MockConsentService) {},
			wantStatus:      http.StatusBadRequest,
		},
		{
			// Criterion 3: no session context → 401.
			name:            "no session context - 401",
			ctxUser:         nil,
			body:            `{"is_adult":true,"consent_recording":true,"consent_tos":true}`,
			setupConsentSvc: func(_ *svcmocks.MockConsentService) {},
			wantStatus:      http.StatusUnauthorized,
		},
		{
			// Criterion 3: wrong type in context → 401.
			name:            "wrong type in context - 401",
			ctxUser:         "not-a-user",
			body:            `{"is_adult":true,"consent_recording":true,"consent_tos":true}`,
			setupConsentSvc: func(_ *svcmocks.MockConsentService) {},
			wantStatus:      http.StatusUnauthorized,
		},
		{
			// Service error propagates as 500.
			name:    "service error - 500",
			ctxUser: domain.User{ID: 42, Email: "alice@example.com"},
			body:    `{"is_adult":true,"consent_recording":true,"consent_tos":true}`,
			setupConsentSvc: func(m *svcmocks.MockConsentService) {
				m.EXPECT().RecordConsent(gomock.Any(), int64(42), allTrue).
					Return(domain.Consent{}, errors.New("db error"))
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			authMock := svcmocks.NewMockAuthService(ctrl)
			consentMock := svcmocks.NewMockConsentService(ctrl)
			tt.setupConsentSvc(consentMock)

			h := newHandler(t, authMock, consentMock)

			e := newTestEcho()
			req := httptest.NewRequest(http.MethodPost, "/v1/auth/consent", strings.NewReader(tt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			if tt.ctxUser != nil {
				c.Set(delivery.UserContextKey, tt.ctxUser)
			}

			_ = h.SubmitConsent(c)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}
