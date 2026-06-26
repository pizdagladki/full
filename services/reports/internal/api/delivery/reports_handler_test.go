package delivery_test

import (
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

	"github.com/pizdagladki/full/services/reports/internal/api/delivery"
	"github.com/pizdagladki/full/services/reports/internal/api/domain"
	"github.com/pizdagladki/full/services/reports/internal/api/service"
	svcmocks "github.com/pizdagladki/full/services/reports/internal/api/service/mocks"
)

// echoValidator wraps validator/v10 to satisfy echo.Validator.
type echoValidator struct{ v *validator.Validate }

func (ev *echoValidator) Validate(i any) error { return ev.v.Struct(i) }

func newEcho(h delivery.ReportsHandler) *echo.Echo {
	e := echo.New()
	e.Validator = &echoValidator{v: validator.New()}
	e.HideBanner = true

	e.POST("/v1/reports/cheat", h.PostCheatReport)
	e.GET("/v1/reports/cooldown/:user_id", h.GetCooldown)

	return e
}

func newHandler(t *testing.T) (*svcmocks.MockReportsService, delivery.ReportsHandler) {
	t.Helper()

	ctrl := gomock.NewController(t)
	svc := svcmocks.NewMockReportsService(ctrl)
	h := delivery.NewReportsHandler(svc, zap.NewNop())

	return svc, h
}

func TestPostCheatReport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		setupMock  func(svc *svcmocks.MockReportsService)
		wantStatus int
	}{
		{
			// criterion: 1 — valid report returns 201
			name: "valid report returns 201",
			body: `{"reporter_id":1,"reported_id":2,"match_id":"m1"}`,
			setupMock: func(svc *svcmocks.MockReportsService) {
				svc.EXPECT().
					ReportCheat(gomock.Any(), int64(1), int64(2), "m1").
					Return(nil)
			},
			wantStatus: http.StatusCreated,
		},
		{
			// criterion: 1 — documented body without reporter_id returns 201
			name: "documented body without reporter_id returns 201",
			body: `{"reported_id":2,"match_id":"m1"}`,
			setupMock: func(svc *svcmocks.MockReportsService) {
				svc.EXPECT().
					ReportCheat(gomock.Any(), int64(0), int64(2), "m1").
					Return(nil)
			},
			wantStatus: http.StatusCreated,
		},
		{
			// criterion: 4 — self-report returns 400
			name: "self-report returns 400",
			body: `{"reporter_id":1,"reported_id":1,"match_id":"m1"}`,
			setupMock: func(svc *svcmocks.MockReportsService) {
				svc.EXPECT().
					ReportCheat(gomock.Any(), int64(1), int64(1), "m1").
					Return(service.ErrSelfReport)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 4 — malformed JSON returns 400
			name:       "malformed body returns 400",
			body:       `{not valid json`,
			setupMock:  func(svc *svcmocks.MockReportsService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 4 — missing required field returns 400
			name:       "missing match_id returns 400",
			body:       `{"reporter_id":1,"reported_id":2}`,
			setupMock:  func(svc *svcmocks.MockReportsService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 1 — idempotent re-report returns 201
			name: "idempotent re-report returns 201",
			body: `{"reporter_id":1,"reported_id":2,"match_id":"m1"}`,
			setupMock: func(svc *svcmocks.MockReportsService) {
				svc.EXPECT().
					ReportCheat(gomock.Any(), int64(1), int64(2), "m1").
					Return(nil) // ON CONFLICT DO NOTHING — service returns nil
			},
			wantStatus: http.StatusCreated,
		},
		{
			// criterion: 5 — internal service error returns 500
			name: "service error returns 500",
			body: `{"reporter_id":1,"reported_id":2,"match_id":"m1"}`,
			setupMock: func(svc *svcmocks.MockReportsService) {
				svc.EXPECT().
					ReportCheat(gomock.Any(), int64(1), int64(2), "m1").
					Return(errors.New("db down"))
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc, h := newHandler(t)
			tt.setupMock(svc)

			e := newEcho(h)
			req := httptest.NewRequest(http.MethodPost, "/v1/reports/cheat", strings.NewReader(tt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestGetCooldown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		userID     string
		setupMock  func(svc *svcmocks.MockReportsService)
		wantStatus int
		wantActive bool
		wantSecs   int
	}{
		{
			// criterion: 3 — cooldown active returns 200 with seconds
			name:   "cooldown active returns 200",
			userID: "42",
			setupMock: func(svc *svcmocks.MockReportsService) {
				svc.EXPECT().
					GetCooldown(gomock.Any(), int64(42)).
					Return(domain.CooldownStatus{Active: true, SecondsRemaining: 500}, nil)
			},
			wantStatus: http.StatusOK,
			wantActive: true,
			wantSecs:   500,
		},
		{
			// criterion: 3 — cooldown inactive returns 200 with active=false
			name:   "cooldown inactive returns 200",
			userID: "42",
			setupMock: func(svc *svcmocks.MockReportsService) {
				svc.EXPECT().
					GetCooldown(gomock.Any(), int64(42)).
					Return(domain.CooldownStatus{Active: false, SecondsRemaining: 0}, nil)
			},
			wantStatus: http.StatusOK,
			wantActive: false,
			wantSecs:   0,
		},
		{
			// criterion: 3 — invalid user_id returns 400
			name:       "invalid user_id returns 400",
			userID:     "not-a-number",
			setupMock:  func(svc *svcmocks.MockReportsService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 3 — service error returns 500
			name:   "service error returns 500",
			userID: "42",
			setupMock: func(svc *svcmocks.MockReportsService) {
				svc.EXPECT().
					GetCooldown(gomock.Any(), int64(42)).
					Return(domain.CooldownStatus{}, errors.New("redis down"))
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc, h := newHandler(t)
			tt.setupMock(svc)

			e := newEcho(h)
			req := httptest.NewRequest(http.MethodGet, "/v1/reports/cooldown/"+tt.userID, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantStatus == http.StatusOK {
				var resp struct {
					Active           bool `json:"active"`
					SecondsRemaining int  `json:"seconds_remaining"`
				}
				if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
					t.Fatalf("decode response: %v", err)
				}

				if resp.Active != tt.wantActive {
					t.Errorf("active = %v, want %v", resp.Active, tt.wantActive)
				}

				if resp.SecondsRemaining != tt.wantSecs {
					t.Errorf("seconds_remaining = %d, want %d", resp.SecondsRemaining, tt.wantSecs)
				}
			}
		})
	}
}
