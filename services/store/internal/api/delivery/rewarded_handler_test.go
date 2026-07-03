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

	"github.com/pizdagladki/full/services/store/internal/api/delivery"
	"github.com/pizdagladki/full/services/store/internal/api/domain"
	svcmocks "github.com/pizdagladki/full/services/store/internal/api/service/mocks"
)

func TestRewardedHandler_Grant(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		userID     any // set in context; nil means not set (unauthenticated)
		body       string
		setupSvc   func(m *svcmocks.MockRewardedService)
		wantStatus int
		wantQty    *int // non-nil: assert quantity in response body
	}{
		{
			// criterion: 1 — a successful grant returns 200 with the new quantity.
			name:   "success returns 200 with quantity",
			userID: int64(5),
			body:   `{"product_id":10}`,
			setupSvc: func(m *svcmocks.MockRewardedService) {
				m.EXPECT().
					GrantFreeDistraction(gomock.Any(), int64(5), int64(10)).
					Return(3, nil)
			},
			wantStatus: http.StatusOK,
			wantQty:    intPtr(3),
		},
		{
			// criterion: 2 — a non-eligible product (paid, or edit) returns 400 and
			// nothing is granted (the mock records no further calls).
			name:   "non-eligible product returns 400",
			userID: int64(5),
			body:   `{"product_id":11}`,
			setupSvc: func(m *svcmocks.MockRewardedService) {
				m.EXPECT().
					GrantFreeDistraction(gomock.Any(), int64(5), int64(11)).
					Return(0, domain.ErrNotGrantable)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 3 — a rate-limited grant returns 429.
			name:   "rate limited returns 429",
			userID: int64(5),
			body:   `{"product_id":10}`,
			setupSvc: func(m *svcmocks.MockRewardedService) {
				m.EXPECT().
					GrantFreeDistraction(gomock.Any(), int64(5), int64(10)).
					Return(0, domain.ErrRateLimited)
			},
			wantStatus: http.StatusTooManyRequests,
		},
		{
			// criterion: 4 — an unknown product_id returns 404.
			name:   "unknown product returns 404",
			userID: int64(5),
			body:   `{"product_id":99}`,
			setupSvc: func(m *svcmocks.MockRewardedService) {
				m.EXPECT().
					GrantFreeDistraction(gomock.Any(), int64(5), int64(99)).
					Return(0, domain.ErrProductNotFound)
			},
			wantStatus: http.StatusNotFound,
		},
		{
			// criterion: 4 — an unauthenticated caller (no userID in context) returns
			// 401 and the service is never called.
			name:       "unauthenticated returns 401",
			userID:     nil,
			body:       `{"product_id":10}`,
			setupSvc:   func(m *svcmocks.MockRewardedService) {},
			wantStatus: http.StatusUnauthorized,
		},
		{
			// a malformed body fails Bind and returns 400 before the service is called.
			name:       "bad json body returns 400",
			userID:     int64(5),
			body:       `{bad json`,
			setupSvc:   func(m *svcmocks.MockRewardedService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			// a missing product_id fails validation and returns 400 before the
			// service is called.
			name:       "missing product_id returns 400",
			userID:     int64(5),
			body:       `{}`,
			setupSvc:   func(m *svcmocks.MockRewardedService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "internal error returns 500",
			userID: int64(5),
			body:   `{"product_id":10}`,
			setupSvc: func(m *svcmocks.MockRewardedService) {
				m.EXPECT().
					GrantFreeDistraction(gomock.Any(), int64(5), int64(10)).
					Return(0, errors.New("db exploded"))
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockSvc := svcmocks.NewMockRewardedService(ctrl)
			tt.setupSvc(mockSvc)

			h := delivery.NewRewardedHandler(mockSvc, zap.NewNop())

			e := newEcho()
			req := httptest.NewRequest(http.MethodPost, "/v1/store/rewarded/grant",
				strings.NewReader(tt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			if tt.userID != nil {
				c.Set(delivery.UserIDContextKey, tt.userID)
			}

			_ = h.Grant(c)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantQty != nil {
				var resp map[string]any
				if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
					t.Fatalf("unmarshal response: %v", err)
				}

				got, ok := resp["quantity"].(float64)
				if !ok || int(got) != *tt.wantQty {
					t.Errorf("quantity = %v, want %d", resp["quantity"], *tt.wantQty)
				}
			}
		})
	}
}

func intPtr(v int) *int { return &v }
