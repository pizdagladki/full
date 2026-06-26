package delivery_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/store/internal/api/delivery"
	"github.com/pizdagladki/full/services/store/internal/api/domain"
	svcmocks "github.com/pizdagladki/full/services/store/internal/api/service/mocks"
)

// customValidator wraps go-playground/validator for Echo.
type customValidator struct {
	v *validator.Validate
}

func (cv *customValidator) Validate(i any) error {
	return cv.v.Struct(i)
}

func newEcho() *echo.Echo {
	e := echo.New()
	e.Validator = &customValidator{v: validator.New()}

	return e
}

func TestPurchaseHandler_CreatePurchase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		userID     any    // set in context; nil means not set (unauthenticated)
		body       string // raw JSON body
		setupSvc   func(m *svcmocks.MockPurchaseService)
		wantStatus int
		wantSecret string // non-empty: assert client_secret in response
	}{
		{
			// criterion: 13 — CreatePurchase returns 201 with client_secret on success
			name:   "success returns 201 with client_secret",
			userID: int64(5),
			body:   `{"product_id":10}`,
			setupSvc: func(m *svcmocks.MockPurchaseService) {
				m.EXPECT().
					InitiatePurchase(gomock.Any(), int64(5), int64(10)).
					Return("cs_test", nil)
			},
			wantStatus: http.StatusCreated,
			wantSecret: "cs_test",
		},
		{
			// criterion: 14 — CreatePurchase returns 401 when user not authenticated
			name:       "unauthenticated returns 401",
			userID:     nil,
			body:       `{"product_id":10}`,
			setupSvc:   func(m *svcmocks.MockPurchaseService) {},
			wantStatus: http.StatusUnauthorized,
		},
		{
			// criterion: 14 — CreatePurchase returns 400 for malformed body
			name:       "bad json body returns 400",
			userID:     int64(5),
			body:       `{bad json`,
			setupSvc:   func(m *svcmocks.MockPurchaseService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 14 — CreatePurchase returns 400 for missing product_id
			name:       "missing product_id returns 400",
			userID:     int64(5),
			body:       `{}`,
			setupSvc:   func(m *svcmocks.MockPurchaseService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 15 — CreatePurchase returns 404 when product not found
			name:   "product not found returns 404",
			userID: int64(5),
			body:   `{"product_id":99}`,
			setupSvc: func(m *svcmocks.MockPurchaseService) {
				m.EXPECT().
					InitiatePurchase(gomock.Any(), int64(5), int64(99)).
					Return("", domain.ErrProductNotFound)
			},
			wantStatus: http.StatusNotFound,
		},
		{
			// criterion: 16 — CreatePurchase returns 409 when product already owned
			name:   "already owned returns 409",
			userID: int64(5),
			body:   `{"product_id":20}`,
			setupSvc: func(m *svcmocks.MockPurchaseService) {
				m.EXPECT().
					InitiatePurchase(gomock.Any(), int64(5), int64(20)).
					Return("", domain.ErrAlreadyOwned)
			},
			wantStatus: http.StatusConflict,
		},
		{
			// criterion: 17 — CreatePurchase returns 500 on internal error
			name:   "internal error returns 500",
			userID: int64(5),
			body:   `{"product_id":10}`,
			setupSvc: func(m *svcmocks.MockPurchaseService) {
				m.EXPECT().
					InitiatePurchase(gomock.Any(), int64(5), int64(10)).
					Return("", errors.New("db exploded"))
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockSvc := svcmocks.NewMockPurchaseService(ctrl)
			tt.setupSvc(mockSvc)

			h := delivery.NewPurchaseHandler(mockSvc, zap.NewNop())

			e := newEcho()
			req := httptest.NewRequest(http.MethodPost, "/v1/store/purchase",
				strings.NewReader(tt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			if tt.userID != nil {
				c.Set(delivery.UserIDContextKey, tt.userID)
			}

			_ = h.CreatePurchase(c)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantSecret != "" {
				var resp map[string]any
				if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
					t.Fatalf("unmarshal response: %v", err)
				}

				got, ok := resp["client_secret"].(string)
				if !ok || got != tt.wantSecret {
					t.Errorf("client_secret = %q, want %q", got, tt.wantSecret)
				}
			}
		})
	}
}

func TestPurchaseHandler_StripeWebhook(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"id":"evt_test","type":"payment_intent.succeeded"}`)
	sigHeader := "t=1,v1=abc"

	tests := []struct {
		name       string
		payload    []byte
		sigHeader  string
		setupSvc   func(m *svcmocks.MockPurchaseService)
		wantStatus int
	}{
		{
			// criterion: 18 — StripeWebhook returns 200 on success
			name:      "success returns 200",
			payload:   payload,
			sigHeader: sigHeader,
			setupSvc: func(m *svcmocks.MockPurchaseService) {
				m.EXPECT().
					HandleWebhook(gomock.Any(), payload, sigHeader).
					Return(nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			// criterion: 19 — StripeWebhook returns 400 for invalid webhook signature
			name:      "invalid webhook signature returns 400",
			payload:   payload,
			sigHeader: "bad-sig",
			setupSvc: func(m *svcmocks.MockPurchaseService) {
				m.EXPECT().
					HandleWebhook(gomock.Any(), payload, "bad-sig").
					Return(domain.ErrInvalidWebhook)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 20 — StripeWebhook returns 500 on internal error
			name:      "internal error returns 500",
			payload:   payload,
			sigHeader: sigHeader,
			setupSvc: func(m *svcmocks.MockPurchaseService) {
				m.EXPECT().
					HandleWebhook(gomock.Any(), payload, sigHeader).
					Return(errors.New("db down"))
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockSvc := svcmocks.NewMockPurchaseService(ctrl)
			tt.setupSvc(mockSvc)

			h := delivery.NewPurchaseHandler(mockSvc, zap.NewNop())

			e := newEcho()
			req := httptest.NewRequest(http.MethodPost, "/v1/store/stripe/webhook",
				bytes.NewReader(tt.payload))
			req.Header.Set("Stripe-Signature", tt.sigHeader)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			_ = h.StripeWebhook(c)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}
