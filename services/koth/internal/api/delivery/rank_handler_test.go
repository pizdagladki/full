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

	"github.com/pizdagladki/full/services/koth/internal/api/delivery"
	"github.com/pizdagladki/full/services/koth/internal/api/domain"
	svcmocks "github.com/pizdagladki/full/services/koth/internal/api/service/mocks"
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

// TestRankHandler_SubmitAttempt verifies criteria 1 and 5: attempts compute
// the achieved/current rank and whether it is newly reached; a non-positive
// held_ms returns 400; an unauthenticated caller returns 401.
func TestRankHandler_SubmitAttempt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		userID     any
		body       string
		setupSvc   func(m *svcmocks.MockRankService)
		wantStatus int
	}{
		{
			// criterion: 1 — a successful attempt returns 200 with the achieved/current rank and newly_reached
			name:   "success returns 200 with rank result",
			userID: int64(5),
			body:   `{"held_ms":16000}`,
			setupSvc: func(m *svcmocks.MockRankService) {
				m.EXPECT().
					SubmitAttempt(gomock.Any(), int64(5), 16000).
					Return(domain.AttemptResult{AchievedRank: 2, CurrentRank: 2, NewlyReached: true}, nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			// criterion: 5 — unauthenticated (no user id in context) returns 401
			name:       "unauthenticated returns 401",
			userID:     nil,
			body:       `{"held_ms":16000}`,
			setupSvc:   func(_ *svcmocks.MockRankService) {},
			wantStatus: http.StatusUnauthorized,
		},
		{
			// criterion: 5 — non-positive held_ms (zero) returns 400
			name:       "zero held_ms returns 400",
			userID:     int64(5),
			body:       `{"held_ms":0}`,
			setupSvc:   func(_ *svcmocks.MockRankService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 5 — non-positive held_ms (negative) returns 400
			name:       "negative held_ms returns 400",
			userID:     int64(5),
			body:       `{"held_ms":-500}`,
			setupSvc:   func(_ *svcmocks.MockRankService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "malformed body returns 400",
			userID:     int64(5),
			body:       `{bad json`,
			setupSvc:   func(_ *svcmocks.MockRankService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "service invalid hold ms error returns 400",
			userID: int64(5),
			body:   `{"held_ms":1}`,
			setupSvc: func(m *svcmocks.MockRankService) {
				m.EXPECT().
					SubmitAttempt(gomock.Any(), int64(5), 1).
					Return(domain.AttemptResult{}, domain.ErrInvalidHoldMs)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "internal error returns 500",
			userID: int64(5),
			body:   `{"held_ms":16000}`,
			setupSvc: func(m *svcmocks.MockRankService) {
				m.EXPECT().
					SubmitAttempt(gomock.Any(), int64(5), 16000).
					Return(domain.AttemptResult{}, errors.New("db exploded"))
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockSvc := svcmocks.NewMockRankService(ctrl)
			tt.setupSvc(mockSvc)

			h := delivery.NewRankHandler(mockSvc, zap.NewNop())

			e := newEcho()
			req := httptest.NewRequest(http.MethodPost, "/v1/koth/ranked/attempt", strings.NewReader(tt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			if tt.userID != nil {
				c.Set(delivery.UserIDContextKey, tt.userID)
			}

			_ = h.SubmitAttempt(c)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

// TestRankHandler_Me verifies criterion: 2 — GET /me returns
// {current_rank, next_target_ms} for the caller and 401 when unauthenticated.
func TestRankHandler_Me(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		userID     any
		setupSvc   func(m *svcmocks.MockRankService)
		wantStatus int
		wantBody   map[string]any
	}{
		{
			// criterion: 2 — returns current_rank + next_target_ms for the caller
			name:   "success returns current rank and next target",
			userID: int64(7),
			setupSvc: func(m *svcmocks.MockRankService) {
				m.EXPECT().Me(gomock.Any(), int64(7)).
					Return(domain.MeResult{CurrentRank: 1, NextTargetMs: 15000}, nil)
			},
			wantStatus: http.StatusOK,
			wantBody:   map[string]any{"current_rank": float64(1), "next_target_ms": float64(15000)},
		},
		{
			// criterion: 5 — unauthenticated returns 401
			name:       "unauthenticated returns 401",
			userID:     nil,
			setupSvc:   func(_ *svcmocks.MockRankService) {},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:   "internal error returns 500",
			userID: int64(7),
			setupSvc: func(m *svcmocks.MockRankService) {
				m.EXPECT().Me(gomock.Any(), int64(7)).
					Return(domain.MeResult{}, errors.New("db exploded"))
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockSvc := svcmocks.NewMockRankService(ctrl)
			tt.setupSvc(mockSvc)

			h := delivery.NewRankHandler(mockSvc, zap.NewNop())

			e := newEcho()
			req := httptest.NewRequest(http.MethodGet, "/v1/koth/ranked/me", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			if tt.userID != nil {
				c.Set(delivery.UserIDContextKey, tt.userID)
			}

			_ = h.Me(c)

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

// TestRankHandler_Leaderboard verifies criterion: 3 — GET /leaderboard is
// public (no auth needed) and returns the accounts-per-rank distribution.
func TestRankHandler_Leaderboard(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setupSvc   func(m *svcmocks.MockRankService)
		wantStatus int
		wantLen    int
	}{
		{
			// criterion: 3 — no auth required, returns the rank distribution list
			name: "success returns distribution without auth",
			setupSvc: func(m *svcmocks.MockRankService) {
				m.EXPECT().Leaderboard(gomock.Any()).
					Return([]domain.RankCount{{Rank: 0, Count: 3}, {Rank: 1, Count: 5}}, nil)
			},
			wantStatus: http.StatusOK,
			wantLen:    2,
		},
		{
			name: "internal error returns 500",
			setupSvc: func(m *svcmocks.MockRankService) {
				m.EXPECT().Leaderboard(gomock.Any()).
					Return(nil, errors.New("db exploded"))
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockSvc := svcmocks.NewMockRankService(ctrl)
			tt.setupSvc(mockSvc)

			h := delivery.NewRankHandler(mockSvc, zap.NewNop())

			e := newEcho()
			// No UserIDContextKey set — this endpoint must not require it.
			req := httptest.NewRequest(http.MethodGet, "/v1/koth/ranked/leaderboard", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			_ = h.Leaderboard(c)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantLen > 0 {
				var got []map[string]any
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("unmarshal response: %v", err)
				}

				if len(got) != tt.wantLen {
					t.Errorf("len(leaderboard) = %d, want %d", len(got), tt.wantLen)
				}
			}
		})
	}
}
