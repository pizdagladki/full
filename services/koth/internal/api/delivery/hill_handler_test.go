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

	"github.com/pizdagladki/full/services/koth/internal/api/delivery"
	"github.com/pizdagladki/full/services/koth/internal/api/domain"
	"github.com/pizdagladki/full/services/koth/internal/api/repository"
	svcmocks "github.com/pizdagladki/full/services/koth/internal/api/service/mocks"
)

// TestHillHandler_CurrentKing verifies criterion: 1 — GET .../:hill_type/king
// is public, returns the current king, 400 on an invalid hill_type, and 404
// when the hill needs seeding.
func TestHillHandler_CurrentKing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		hillType   string
		setupSvc   func(m *svcmocks.MockHillService)
		wantStatus int
		wantBody   map[string]any
	}{
		{
			// criterion: 1 — a seeded hill returns 200 with {user_id, clip_id, blink_ts_ms}
			name:     "success returns current king",
			hillType: "daily",
			setupSvc: func(m *svcmocks.MockHillService) {
				m.EXPECT().CurrentKing(gomock.Any(), "daily").
					Return(domain.KingReign{UserID: 42, ClipID: "clip-1", BlinkTsMs: 8000}, nil)
			},
			wantStatus: http.StatusOK,
			wantBody:   map[string]any{"user_id": float64(42), "clip_id": "clip-1", "blink_ts_ms": float64(8000)},
		},
		{
			// criterion: 1 — no current reign (needs seeding) returns 404
			name:     "unseeded hill returns 404",
			hillType: "monthly",
			setupSvc: func(m *svcmocks.MockHillService) {
				m.EXPECT().CurrentKing(gomock.Any(), "monthly").
					Return(domain.KingReign{}, repository.ErrHillNotFound)
			},
			wantStatus: http.StatusNotFound,
		},
		{
			// criterion: 5 — an invalid hill_type path param returns 400
			name:     "invalid hill_type returns 400",
			hillType: "weekly",
			setupSvc: func(m *svcmocks.MockHillService) {
				m.EXPECT().CurrentKing(gomock.Any(), "weekly").
					Return(domain.KingReign{}, domain.ErrInvalidHillType)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:     "internal error returns 500",
			hillType: "daily",
			setupSvc: func(m *svcmocks.MockHillService) {
				m.EXPECT().CurrentKing(gomock.Any(), "daily").
					Return(domain.KingReign{}, errors.New("db exploded"))
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockSvc := svcmocks.NewMockHillService(ctrl)
			tt.setupSvc(mockSvc)

			h := delivery.NewHillHandler(mockSvc, zap.NewNop())

			e := newEcho()
			req := httptest.NewRequest(http.MethodGet, "/v1/koth/hills/"+tt.hillType+"/king", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("hill_type")
			c.SetParamValues(tt.hillType)

			// No UserIDContextKey set — this endpoint must not require it.
			_ = h.CurrentKing(c)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantBody != nil {
				var got map[string]any
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("unmarshal response: %v", err)
				}

				for k, v := range tt.wantBody {
					if got[k] != v {
						t.Errorf("body[%q] = %v, want %v", k, got[k], v)
					}
				}
			}
		})
	}
}

// TestHillHandler_Challenge verifies criteria 2, 3, 5, and 6 — POST
// .../:hill_type/challenge requires auth, validates the body and hill_type,
// and returns the decided won/current-king outcome.
func TestHillHandler_Challenge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		hillType   string
		userID     any
		body       string
		setupSvc   func(m *svcmocks.MockHillService)
		wantStatus int
		wantBody   map[string]any
	}{
		{
			// criterion: 2 — a winning challenge returns 200 with won=true and the new king
			name:     "challenger wins returns 200 won true",
			hillType: "daily",
			userID:   int64(99),
			body:     `{"survived_ms":9000,"new_clip_id":"clip-new"}`,
			setupSvc: func(m *svcmocks.MockHillService) {
				m.EXPECT().Challenge(gomock.Any(), "daily", int64(99), 9000, "clip-new").
					Return(domain.ChallengeOutcome{
						Won:  true,
						King: domain.KingReign{UserID: 99, ClipID: "clip-new", BlinkTsMs: 9000},
					}, nil)
			},
			wantStatus: http.StatusOK,
			wantBody:   map[string]any{"won": true},
		},
		{
			// criterion: 2 — a losing challenge returns 200 with won=false and the current king unchanged
			name:     "challenger loses returns 200 won false",
			hillType: "daily",
			userID:   int64(99),
			body:     `{"survived_ms":5000,"new_clip_id":"clip-new"}`,
			setupSvc: func(m *svcmocks.MockHillService) {
				m.EXPECT().Challenge(gomock.Any(), "daily", int64(99), 5000, "clip-new").
					Return(domain.ChallengeOutcome{
						Won:  false,
						King: domain.KingReign{UserID: 42, ClipID: "clip-1", BlinkTsMs: 8000},
					}, nil)
			},
			wantStatus: http.StatusOK,
			wantBody:   map[string]any{"won": false},
		},
		{
			// criterion: 5 — unauthenticated (no user id in context) returns 401
			name:       "unauthenticated returns 401",
			hillType:   "daily",
			userID:     nil,
			body:       `{"survived_ms":9000,"new_clip_id":"clip-new"}`,
			setupSvc:   func(_ *svcmocks.MockHillService) {},
			wantStatus: http.StatusUnauthorized,
		},
		{
			// criterion: 5 — a malformed body (bad JSON) returns 400
			name:       "malformed json body returns 400",
			hillType:   "daily",
			userID:     int64(99),
			body:       `{bad json`,
			setupSvc:   func(_ *svcmocks.MockHillService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 5 — missing new_clip_id returns 400
			name:       "missing new_clip_id returns 400",
			hillType:   "daily",
			userID:     int64(99),
			body:       `{"survived_ms":9000}`,
			setupSvc:   func(_ *svcmocks.MockHillService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 5 — invalid (wrong type) new_clip_id returns 400
			name:       "invalid type new_clip_id returns 400",
			hillType:   "daily",
			userID:     int64(99),
			body:       `{"survived_ms":9000,"new_clip_id":123}`,
			setupSvc:   func(_ *svcmocks.MockHillService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 5 — invalid (negative) survived_ms returns 400
			name:       "negative survived_ms returns 400",
			hillType:   "daily",
			userID:     int64(99),
			body:       `{"survived_ms":-100,"new_clip_id":"clip-new"}`,
			setupSvc:   func(_ *svcmocks.MockHillService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 5 — an invalid hill_type path param returns 400
			name:     "invalid hill_type returns 400",
			hillType: "weekly",
			userID:   int64(99),
			body:     `{"survived_ms":9000,"new_clip_id":"clip-new"}`,
			setupSvc: func(m *svcmocks.MockHillService) {
				m.EXPECT().Challenge(gomock.Any(), "weekly", int64(99), 9000, "clip-new").
					Return(domain.ChallengeOutcome{}, domain.ErrInvalidHillType)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 6 — seeding-404: challenging an unseeded hill returns 404
			name:     "unseeded hill returns 404",
			hillType: "monthly",
			userID:   int64(99),
			body:     `{"survived_ms":9000,"new_clip_id":"clip-new"}`,
			setupSvc: func(m *svcmocks.MockHillService) {
				m.EXPECT().Challenge(gomock.Any(), "monthly", int64(99), 9000, "clip-new").
					Return(domain.ChallengeOutcome{}, repository.ErrHillNotFound)
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:     "internal error returns 500",
			hillType: "daily",
			userID:   int64(99),
			body:     `{"survived_ms":9000,"new_clip_id":"clip-new"}`,
			setupSvc: func(m *svcmocks.MockHillService) {
				m.EXPECT().Challenge(gomock.Any(), "daily", int64(99), 9000, "clip-new").
					Return(domain.ChallengeOutcome{}, errors.New("db exploded"))
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockSvc := svcmocks.NewMockHillService(ctrl)
			tt.setupSvc(mockSvc)

			h := delivery.NewHillHandler(mockSvc, zap.NewNop())

			e := newEcho()
			req := httptest.NewRequest(
				http.MethodPost, "/v1/koth/hills/"+tt.hillType+"/challenge", strings.NewReader(tt.body),
			)
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("hill_type")
			c.SetParamValues(tt.hillType)

			if tt.userID != nil {
				c.Set(delivery.UserIDContextKey, tt.userID)
			}

			_ = h.Challenge(c)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantBody != nil {
				var got map[string]any
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("unmarshal response: %v", err)
				}

				for k, v := range tt.wantBody {
					if got[k] != v {
						t.Errorf("body[%q] = %v, want %v", k, got[k], v)
					}
				}
			}
		})
	}
}
