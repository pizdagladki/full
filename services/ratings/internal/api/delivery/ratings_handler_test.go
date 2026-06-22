package delivery

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
