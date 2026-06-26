package delivery

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

	"github.com/pizdagladki/full/services/ratings/internal/api/domain"
	"github.com/pizdagladki/full/services/ratings/internal/api/service"
	svcmocks "github.com/pizdagladki/full/services/ratings/internal/api/service/mocks"
)

// echoValidator adapts validator/v10 to echo.Validator.
type echoValidator struct{ v *validator.Validate }

func (ev *echoValidator) Validate(i any) error { return ev.v.Struct(i) }

// newTestEcho wires a handler into a fresh Echo instance and returns it.
// Using e.ServeHTTP lets Echo's default error handler convert *HTTPError → status code.
func newTestEcho(h RatingsHandler) *echo.Echo {
	e := echo.New()
	e.Validator = &echoValidator{v: validator.New()}
	e.POST("/v1/matches/result", h.PostMatchResult)
	e.GET("/v1/ratings/:user_id", h.GetRating)
	e.GET("/v1/matches/history", h.GetMatchHistory)

	return e
}

// ─── PostMatchResult ──────────────────────────────────────────────────────────

func TestPostMatchResult(t *testing.T) {
	t.Parallel()

	dur := 4000

	tests := []struct {
		name       string
		body       string
		setup      func(svc *svcmocks.MockRatingsService)
		wantStatus int
		wantBody   func(t *testing.T, body string)
	}{
		{
			name: "200 happy path",
			body: `{"winner_id":1,"loser_id":2,"mode":"classic","duration_ms":4000}`,
			setup: func(svc *svcmocks.MockRatingsService) {
				svc.EXPECT().ApplyMatchResult(gomock.Any(), domain.MatchInput{
					WinnerID:   1,
					LoserID:    2,
					Mode:       "classic",
					DurationMS: &dur,
				}).Return(domain.MatchResult{
					Winner:      domain.Rating{UserID: 1, ELO: 1016, Level: 4, GamesPlayed: 1},
					Loser:       domain.Rating{UserID: 2, ELO: 987, Level: 4, GamesPlayed: 1},
					WinnerDelta: 16,
					LoserDelta:  -13,
				}, nil)
			},
			wantStatus: http.StatusOK,
			wantBody: func(t *testing.T, body string) {
				t.Helper()

				var resp matchResultResponse
				if err := json.Unmarshal([]byte(body), &resp); err != nil {
					t.Fatalf("decode response: %v", err)
				}

				if resp.Winner.ELO != 1016 {
					t.Errorf("winner.elo = %d, want 1016", resp.Winner.ELO)
				}

				if resp.Winner.ELODelta != 16 {
					t.Errorf("winner.elo_delta = %d, want 16", resp.Winner.ELODelta)
				}

				if resp.Loser.ELODelta != -13 {
					t.Errorf("loser.elo_delta = %d, want -13", resp.Loser.ELODelta)
				}
			},
		},
		{
			name:       "400 malformed JSON",
			body:       `{not json`,
			setup:      func(_ *svcmocks.MockRatingsService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "400 missing required field mode",
			body:       `{"winner_id":1,"loser_id":2}`,
			setup:      func(_ *svcmocks.MockRatingsService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "400 missing winner_id",
			body:       `{"loser_id":2,"mode":"classic"}`,
			setup:      func(_ *svcmocks.MockRatingsService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "400 winner_id is zero",
			body:       `{"winner_id":0,"loser_id":2,"mode":"classic"}`,
			setup:      func(_ *svcmocks.MockRatingsService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "400 winner_id is negative",
			body:       `{"winner_id":-1,"loser_id":2,"mode":"classic"}`,
			setup:      func(_ *svcmocks.MockRatingsService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "400 loser_id is zero",
			body:       `{"winner_id":1,"loser_id":0,"mode":"classic"}`,
			setup:      func(_ *svcmocks.MockRatingsService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "400 loser_id is negative",
			body:       `{"winner_id":1,"loser_id":-5,"mode":"classic"}`,
			setup:      func(_ *svcmocks.MockRatingsService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "400 mode too long",
			body:       `{"winner_id":1,"loser_id":2,"mode":"` + strings.Repeat("x", 65) + `"}`,
			setup:      func(_ *svcmocks.MockRatingsService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "400 negative duration_ms",
			body:       `{"winner_id":1,"loser_id":2,"mode":"classic","duration_ms":-1}`,
			setup:      func(_ *svcmocks.MockRatingsService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "400 same player from service",
			body: `{"winner_id":5,"loser_id":5,"mode":"classic"}`,
			setup: func(svc *svcmocks.MockRatingsService) {
				svc.EXPECT().ApplyMatchResult(gomock.Any(), gomock.Any()).
					Return(domain.MatchResult{}, service.ErrSamePlayer)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "500 internal repo error",
			body: `{"winner_id":1,"loser_id":2,"mode":"classic"}`,
			setup: func(svc *svcmocks.MockRatingsService) {
				svc.EXPECT().ApplyMatchResult(gomock.Any(), gomock.Any()).
					Return(domain.MatchResult{}, errors.New("db down"))
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			svc := svcmocks.NewMockRatingsService(ctrl)
			tt.setup(svc)

			h := NewRatingsHandler(svc, zap.NewNop())
			e := newTestEcho(h)

			req := httptest.NewRequest(http.MethodPost, "/v1/matches/result",
				strings.NewReader(tt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantBody != nil {
				tt.wantBody(t, rec.Body.String())
			}
		})
	}
}

// ─── GetMatchHistory ──────────────────────────────────────────────────────────

func TestGetMatchHistory(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	dur := 4000
	winnerDelta := 32
	loserDelta := -26

	tests := []struct {
		name       string
		query      string
		setup      func(svc *svcmocks.MockRatingsService)
		wantStatus int
		wantBody   func(t *testing.T, body string)
	}{
		{
			// criterion: 3 — missing user_id → 400
			name:       "400 missing user_id",
			query:      "/v1/matches/history",
			setup:      func(_ *svcmocks.MockRatingsService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 3 — non-numeric user_id → 400
			name:       "400 non-numeric user_id",
			query:      "/v1/matches/history?user_id=abc",
			setup:      func(_ *svcmocks.MockRatingsService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 3 — negative user_id → 400
			name:       "400 negative user_id",
			query:      "/v1/matches/history?user_id=-1",
			setup:      func(_ *svcmocks.MockRatingsService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 3 — zero user_id → 400
			name:       "400 zero user_id",
			query:      "/v1/matches/history?user_id=0",
			setup:      func(_ *svcmocks.MockRatingsService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 3 — non-numeric limit → 400
			name:       "400 non-numeric limit",
			query:      "/v1/matches/history?user_id=1&limit=abc",
			setup:      func(_ *svcmocks.MockRatingsService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 3 — negative limit → 400
			name:       "400 negative limit",
			query:      "/v1/matches/history?user_id=1&limit=-1",
			setup:      func(_ *svcmocks.MockRatingsService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 3 — non-numeric offset → 400
			name:       "400 non-numeric offset",
			query:      "/v1/matches/history?user_id=1&offset=abc",
			setup:      func(_ *svcmocks.MockRatingsService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 3 — negative offset → 400
			name:       "400 negative offset",
			query:      "/v1/matches/history?user_id=1&offset=-5",
			setup:      func(_ *svcmocks.MockRatingsService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			// criterion: 3 — limit > 50 is silently capped to 50
			name:  "200 limit capped to 50",
			query: "/v1/matches/history?user_id=1&limit=100",
			setup: func(svc *svcmocks.MockRatingsService) {
				// The handler must cap to 50 before calling the service.
				svc.EXPECT().ListMatchHistory(gomock.Any(), int64(1), 50, 0).
					Return([]domain.MatchHistoryItem{}, nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			// criterion: 3 — default limit (20) and offset (0) when not supplied
			name:  "200 default limit and offset",
			query: "/v1/matches/history?user_id=5",
			setup: func(svc *svcmocks.MockRatingsService) {
				svc.EXPECT().ListMatchHistory(gomock.Any(), int64(5), 20, 0).
					Return([]domain.MatchHistoryItem{}, nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			// criterion: 2 — win result with correct (winner's) elo_delta
			name:  "200 win result and winner elo_delta",
			query: "/v1/matches/history?user_id=1",
			setup: func(svc *svcmocks.MockRatingsService) {
				svc.EXPECT().ListMatchHistory(gomock.Any(), int64(1), 20, 0).
					Return([]domain.MatchHistoryItem{
						{MatchID: 10, OpponentID: 2, Result: "win", Mode: "classic", ELODelta: winnerDelta, DurationMS: &dur, CreatedAt: now},
					}, nil)
			},
			wantStatus: http.StatusOK,
			wantBody: func(t *testing.T, body string) {
				t.Helper()

				var resp matchHistoryResponse
				if err := json.Unmarshal([]byte(body), &resp); err != nil {
					t.Fatalf("decode: %v", err)
				}

				if len(resp.Matches) != 1 {
					t.Fatalf("len(matches) = %d, want 1", len(resp.Matches))
				}

				if resp.Matches[0].Result != "win" {
					t.Errorf("result = %q, want %q", resp.Matches[0].Result, "win")
				}

				if resp.Matches[0].ELODelta != winnerDelta {
					t.Errorf("elo_delta = %d, want %d (winner's delta)", resp.Matches[0].ELODelta, winnerDelta)
				}
			},
		},
		{
			// criterion: 2 — loss result with correct (loser's) elo_delta
			name:  "200 loss result and loser elo_delta",
			query: "/v1/matches/history?user_id=2",
			setup: func(svc *svcmocks.MockRatingsService) {
				svc.EXPECT().ListMatchHistory(gomock.Any(), int64(2), 20, 0).
					Return([]domain.MatchHistoryItem{
						{MatchID: 10, OpponentID: 1, Result: "loss", Mode: "classic", ELODelta: loserDelta, DurationMS: nil, CreatedAt: now},
					}, nil)
			},
			wantStatus: http.StatusOK,
			wantBody: func(t *testing.T, body string) {
				t.Helper()

				var resp matchHistoryResponse
				if err := json.Unmarshal([]byte(body), &resp); err != nil {
					t.Fatalf("decode: %v", err)
				}

				if len(resp.Matches) != 1 {
					t.Fatalf("len(matches) = %d, want 1", len(resp.Matches))
				}

				if resp.Matches[0].Result != "loss" {
					t.Errorf("result = %q, want %q", resp.Matches[0].Result, "loss")
				}

				if resp.Matches[0].ELODelta != loserDelta {
					t.Errorf("elo_delta = %d, want %d (loser's delta)", resp.Matches[0].ELODelta, loserDelta)
				}
			},
		},
		{
			// criterion: 4 — user with no matches → 200, matches=[] (not null)
			name:  "200 empty list not null",
			query: "/v1/matches/history?user_id=99",
			setup: func(svc *svcmocks.MockRatingsService) {
				svc.EXPECT().ListMatchHistory(gomock.Any(), int64(99), 20, 0).
					Return([]domain.MatchHistoryItem{}, nil)
			},
			wantStatus: http.StatusOK,
			wantBody: func(t *testing.T, body string) {
				t.Helper()

				var resp matchHistoryResponse
				if err := json.Unmarshal([]byte(body), &resp); err != nil {
					t.Fatalf("decode: %v", err)
				}

				if resp.Matches == nil {
					t.Error("matches must not be JSON null — want []")
				}

				if len(resp.Matches) != 0 {
					t.Errorf("len(matches) = %d, want 0", len(resp.Matches))
				}
			},
		},
		{
			// criterion: 5 — service error → 500
			name:  "500 service error",
			query: "/v1/matches/history?user_id=1",
			setup: func(svc *svcmocks.MockRatingsService) {
				svc.EXPECT().ListMatchHistory(gomock.Any(), int64(1), 20, 0).
					Return(nil, errors.New("db timeout"))
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			// criterion: 3 — pagination params wired end-to-end
			name:  "200 explicit limit and offset forwarded to service",
			query: "/v1/matches/history?user_id=1&limit=5&offset=10",
			setup: func(svc *svcmocks.MockRatingsService) {
				svc.EXPECT().ListMatchHistory(gomock.Any(), int64(1), 5, 10).
					Return([]domain.MatchHistoryItem{}, nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			// criterion: 1 — newest-first ordering preserved in response
			name:  "200 newest-first ordering preserved",
			query: "/v1/matches/history?user_id=1",
			setup: func(svc *svcmocks.MockRatingsService) {
				earlier := now.Add(-time.Hour)
				svc.EXPECT().ListMatchHistory(gomock.Any(), int64(1), 20, 0).
					Return([]domain.MatchHistoryItem{
						{MatchID: 20, OpponentID: 3, Result: "win", Mode: "ranked", ELODelta: 16, CreatedAt: now},
						{MatchID: 19, OpponentID: 4, Result: "loss", Mode: "classic", ELODelta: -13, CreatedAt: earlier},
					}, nil)
			},
			wantStatus: http.StatusOK,
			wantBody: func(t *testing.T, body string) {
				t.Helper()

				var resp matchHistoryResponse
				if err := json.Unmarshal([]byte(body), &resp); err != nil {
					t.Fatalf("decode: %v", err)
				}

				if len(resp.Matches) != 2 {
					t.Fatalf("len(matches) = %d, want 2", len(resp.Matches))
				}

				if resp.Matches[0].MatchID != 20 {
					t.Errorf("matches[0].match_id = %d, want 20 (newest first)", resp.Matches[0].MatchID)
				}

				if resp.Matches[1].MatchID != 19 {
					t.Errorf("matches[1].match_id = %d, want 19", resp.Matches[1].MatchID)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			svc := svcmocks.NewMockRatingsService(ctrl)
			tt.setup(svc)

			h := NewRatingsHandler(svc, zap.NewNop())
			e := newTestEcho(h)

			req := httptest.NewRequest(http.MethodGet, tt.query, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantBody != nil {
				tt.wantBody(t, rec.Body.String())
			}
		})
	}
}

// ─── GetRating ────────────────────────────────────────────────────────────────

func TestGetRating(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		userIDParam string
		setup       func(svc *svcmocks.MockRatingsService)
		wantStatus  int
		wantELO     int
	}{
		{
			name:        "200 existing player",
			userIDParam: "42",
			setup: func(svc *svcmocks.MockRatingsService) {
				svc.EXPECT().GetRating(gomock.Any(), int64(42)).
					Return(domain.Rating{UserID: 42, ELO: 1100, Level: 5, GamesPlayed: 25}, nil)
			},
			wantStatus: http.StatusOK,
			wantELO:    1100,
		},
		{
			name:        "200 unknown player returns defaults",
			userIDParam: "99",
			setup: func(svc *svcmocks.MockRatingsService) {
				svc.EXPECT().GetRating(gomock.Any(), int64(99)).
					Return(domain.Rating{
						UserID:      99,
						ELO:         domain.DefaultELO,
						Level:       domain.DefaultLevel,
						GamesPlayed: domain.DefaultGamesPlayed,
					}, nil)
			},
			wantStatus: http.StatusOK,
			wantELO:    domain.DefaultELO,
		},
		{
			name:        "400 non-numeric user_id",
			userIDParam: "abc",
			setup:       func(_ *svcmocks.MockRatingsService) {},
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "400 zero user_id",
			userIDParam: "0",
			setup:       func(_ *svcmocks.MockRatingsService) {},
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "400 negative user_id",
			userIDParam: "-1",
			setup:       func(_ *svcmocks.MockRatingsService) {},
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "500 service error",
			userIDParam: "7",
			setup: func(svc *svcmocks.MockRatingsService) {
				svc.EXPECT().GetRating(gomock.Any(), int64(7)).
					Return(domain.Rating{}, errors.New("timeout"))
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			svc := svcmocks.NewMockRatingsService(ctrl)
			tt.setup(svc)

			h := NewRatingsHandler(svc, zap.NewNop())
			e := newTestEcho(h)

			req := httptest.NewRequest(http.MethodGet, "/v1/ratings/"+tt.userIDParam, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantStatus == http.StatusOK && tt.wantELO != 0 {
				var resp ratingResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
					t.Fatalf("decode response: %v", err)
				}

				if resp.ELO != tt.wantELO {
					t.Errorf("elo = %d, want %d", resp.ELO, tt.wantELO)
				}
			}
		})
	}
}
