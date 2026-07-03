package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/ratings/internal/api/domain"
	repomocks "github.com/pizdagladki/full/services/ratings/internal/api/repository/mocks"
	"github.com/pizdagladki/full/services/ratings/internal/api/service"
	svcmocks "github.com/pizdagladki/full/services/ratings/internal/api/service/mocks"
)

// Points-ledger credit reasons, mirrored here so the test doesn't depend on
// the service package's unexported constants.
const (
	reasonMatchWin = "match_win"
	reasonLevelUp  = "level_up"
)

func TestRatingsService_ApplyMatchResult(t *testing.T) {
	t.Parallel()

	dur := 3000

	tests := []struct {
		name    string
		input   domain.MatchInput
		setup   func(repo *repomocks.MockRatingsRepository)
		points  func(points *svcmocks.MockPointsClient)
		want    domain.MatchResult
		wantErr error
	}{
		{
			name: "happy path delegates to repo",
			input: domain.MatchInput{
				WinnerID:   1,
				LoserID:    2,
				Mode:       "classic",
				DurationMS: &dur,
			},
			setup: func(repo *repomocks.MockRatingsRepository) {
				repo.EXPECT().ApplyMatchResult(gomock.Any(), domain.MatchInput{
					WinnerID:   1,
					LoserID:    2,
					Mode:       "classic",
					DurationMS: &dur,
				}).Return(domain.MatchResult{
					Winner:      domain.Rating{UserID: 1, ELO: 1016, Level: 4, GamesPlayed: 1},
					Loser:       domain.Rating{UserID: 2, ELO: 987, Level: 4, GamesPlayed: 1},
					WinnerDelta: 16,
					LoserDelta:  -13,
					MatchID:     7,
				}, nil)
			},
			points: func(points *svcmocks.MockPointsClient) {
				points.EXPECT().
					Credit(gomock.Any(), service.CreditRequest{UserID: 1, Reason: reasonMatchWin, RefID: "7"}).
					Return(nil)
			},
			want: domain.MatchResult{
				Winner:      domain.Rating{UserID: 1, ELO: 1016, Level: 4, GamesPlayed: 1},
				Loser:       domain.Rating{UserID: 2, ELO: 987, Level: 4, GamesPlayed: 1},
				WinnerDelta: 16,
				LoserDelta:  -13,
				MatchID:     7,
			},
		},
		{
			name: "same player returns ErrSamePlayer — no repo call",
			input: domain.MatchInput{
				WinnerID: 5,
				LoserID:  5,
				Mode:     "classic",
			},
			setup:   func(_ *repomocks.MockRatingsRepository) {},
			wantErr: service.ErrSamePlayer,
		},
		{
			name: "repo error is propagated",
			input: domain.MatchInput{
				WinnerID: 1,
				LoserID:  2,
				Mode:     "classic",
			},
			setup: func(repo *repomocks.MockRatingsRepository) {
				repo.EXPECT().ApplyMatchResult(gomock.Any(), gomock.Any()).
					Return(domain.MatchResult{}, errors.New("db error"))
			},
			wantErr: errors.New("db error"),
		},
		{
			// criterion: 1 — the WINNER (not the loser) is credited match_win exactly once.
			name: "winner credited match_win exactly once — loser not credited",
			input: domain.MatchInput{
				WinnerID: 10,
				LoserID:  20,
				Mode:     "classic",
			},
			setup: func(repo *repomocks.MockRatingsRepository) {
				repo.EXPECT().ApplyMatchResult(gomock.Any(), gomock.Any()).Return(domain.MatchResult{
					Winner:  domain.Rating{UserID: 10, ELO: 1020, Level: 4, GamesPlayed: 1},
					Loser:   domain.Rating{UserID: 20, ELO: 980, Level: 4, GamesPlayed: 1},
					MatchID: 55,
				}, nil)
			},
			points: func(points *svcmocks.MockPointsClient) {
				// Exactly one call, for the winner only — gomock's Times(1) plus asserting
				// the argument's UserID==10 (never 20, the loser) enforces both halves of
				// the criterion. Any call with UserID==20, or a second call, fails the test.
				points.EXPECT().
					Credit(gomock.Any(), service.CreditRequest{UserID: 10, Reason: reasonMatchWin, RefID: "55"}).
					Times(1).
					Return(nil)
			},
			want: domain.MatchResult{
				Winner:  domain.Rating{UserID: 10, ELO: 1020, Level: 4, GamesPlayed: 1},
				Loser:   domain.Rating{UserID: 20, ELO: 980, Level: 4, GamesPlayed: 1},
				MatchID: 55,
			},
		},
		{
			// criterion: 2 — level band increased → an ADDITIONAL level_up credit is sent.
			name: "level-up credited when winner's band increased",
			input: domain.MatchInput{
				WinnerID: 1,
				LoserID:  2,
				Mode:     "classic",
			},
			setup: func(repo *repomocks.MockRatingsRepository) {
				repo.EXPECT().ApplyMatchResult(gomock.Any(), gomock.Any()).Return(domain.MatchResult{
					Winner:          domain.Rating{UserID: 1, ELO: 1108, Level: 5, GamesPlayed: 21},
					Loser:           domain.Rating{UserID: 2, ELO: 1378, Level: 6, GamesPlayed: 21},
					MatchID:         777,
					WinnerLeveledUp: true,
				}, nil)
			},
			points: func(points *svcmocks.MockPointsClient) {
				points.EXPECT().
					Credit(gomock.Any(), service.CreditRequest{UserID: 1, Reason: reasonMatchWin, RefID: "777"}).
					Return(nil)
				points.EXPECT().
					Credit(gomock.Any(), service.CreditRequest{UserID: 1, Reason: reasonLevelUp, RefID: "777:level"}).
					Return(nil)
			},
			want: domain.MatchResult{
				Winner:          domain.Rating{UserID: 1, ELO: 1108, Level: 5, GamesPlayed: 21},
				Loser:           domain.Rating{UserID: 2, ELO: 1378, Level: 6, GamesPlayed: 21},
				MatchID:         777,
				WinnerLeveledUp: true,
			},
		},
		{
			// criterion: 2 — band did NOT change → no level_up credit, only match_win.
			name: "no level-up credit when winner's band did not change",
			input: domain.MatchInput{
				WinnerID: 1,
				LoserID:  2,
				Mode:     "classic",
			},
			setup: func(repo *repomocks.MockRatingsRepository) {
				repo.EXPECT().ApplyMatchResult(gomock.Any(), gomock.Any()).Return(domain.MatchResult{
					Winner:          domain.Rating{UserID: 1, ELO: 1032, Level: 4, GamesPlayed: 1},
					Loser:           domain.Rating{UserID: 2, ELO: 974, Level: 4, GamesPlayed: 1},
					MatchID:         501,
					WinnerLeveledUp: false,
				}, nil)
			},
			points: func(points *svcmocks.MockPointsClient) {
				// Only match_win is registered — gomock fails the test if a second
				// (level_up) call is made, since no expectation covers it.
				points.EXPECT().
					Credit(gomock.Any(), service.CreditRequest{UserID: 1, Reason: reasonMatchWin, RefID: "501"}).
					Times(1).
					Return(nil)
			},
			want: domain.MatchResult{
				Winner:          domain.Rating{UserID: 1, ELO: 1032, Level: 4, GamesPlayed: 1},
				Loser:           domain.Rating{UserID: 2, ELO: 974, Level: 4, GamesPlayed: 1},
				MatchID:         501,
				WinnerLeveledUp: false,
			},
		},
		{
			// criterion: 4 — a PointsClient failure is logged and swallowed: the match
			// result still returns successfully with nil error and unchanged ratings.
			name: "points credit failure is non-blocking",
			input: domain.MatchInput{
				WinnerID: 1,
				LoserID:  2,
				Mode:     "classic",
			},
			setup: func(repo *repomocks.MockRatingsRepository) {
				repo.EXPECT().ApplyMatchResult(gomock.Any(), gomock.Any()).Return(domain.MatchResult{
					Winner:          domain.Rating{UserID: 1, ELO: 1108, Level: 5, GamesPlayed: 21},
					Loser:           domain.Rating{UserID: 2, ELO: 1378, Level: 6, GamesPlayed: 21},
					MatchID:         777,
					WinnerLeveledUp: true,
				}, nil)
			},
			points: func(points *svcmocks.MockPointsClient) {
				points.EXPECT().
					Credit(gomock.Any(), service.CreditRequest{UserID: 1, Reason: reasonMatchWin, RefID: "777"}).
					Return(errors.New("store unreachable"))
				points.EXPECT().
					Credit(gomock.Any(), service.CreditRequest{UserID: 1, Reason: reasonLevelUp, RefID: "777:level"}).
					Return(errors.New("store unreachable"))
			},
			want: domain.MatchResult{
				Winner:          domain.Rating{UserID: 1, ELO: 1108, Level: 5, GamesPlayed: 21},
				Loser:           domain.Rating{UserID: 2, ELO: 1378, Level: 6, GamesPlayed: 21},
				MatchID:         777,
				WinnerLeveledUp: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := repomocks.NewMockRatingsRepository(ctrl)
			tt.setup(repo)

			points := svcmocks.NewMockPointsClient(ctrl)
			if tt.points != nil {
				tt.points(points)
			}

			svc := service.NewRatingsService(repo, zap.NewNop(), points)
			got, err := svc.ApplyMatchResult(context.Background(), tt.input)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("ApplyMatchResult() error = nil, want %v", tt.wantErr)
				}
				// For sentinel errors check exact match; for wrapped errors check wrapping.
				if errors.Is(tt.wantErr, service.ErrSamePlayer) && !errors.Is(err, service.ErrSamePlayer) {
					t.Errorf("ApplyMatchResult() error = %v, want ErrSamePlayer", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("ApplyMatchResult() unexpected error = %v", err)
			}

			if got != tt.want {
				t.Errorf("ApplyMatchResult() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRatingsService_ListMatchHistory(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	dur := 3000

	tests := []struct {
		name    string
		userID  int64
		limit   int
		offset  int
		setup   func(repo *repomocks.MockRatingsRepository)
		want    []domain.MatchHistoryItem
		wantErr bool
	}{
		{
			// criterion: 1,2 — items from repo forwarded with correct fields
			name:   "items returned are forwarded to caller",
			userID: 1,
			limit:  10,
			offset: 0,
			setup: func(repo *repomocks.MockRatingsRepository) {
				repo.EXPECT().ListMatchHistory(gomock.Any(), int64(1), 10, 0).
					Return([]domain.MatchHistoryItem{
						{MatchID: 7, OpponentID: 2, Result: "win", Mode: "classic", ELODelta: 32, DurationMS: &dur, CreatedAt: now},
					}, nil)
			},
			want: []domain.MatchHistoryItem{
				{MatchID: 7, OpponentID: 2, Result: "win", Mode: "classic", ELODelta: 32, DurationMS: &dur, CreatedAt: now},
			},
		},
		{
			// criterion: 4 — empty list forwarded (not nil)
			name:   "empty list forwarded from repo",
			userID: 99,
			limit:  20,
			offset: 0,
			setup: func(repo *repomocks.MockRatingsRepository) {
				repo.EXPECT().ListMatchHistory(gomock.Any(), int64(99), 20, 0).
					Return([]domain.MatchHistoryItem{}, nil)
			},
			want: []domain.MatchHistoryItem{},
		},
		{
			// criterion: 5 — repo error is propagated
			name:   "repo error is propagated",
			userID: 1,
			limit:  10,
			offset: 0,
			setup: func(repo *repomocks.MockRatingsRepository) {
				repo.EXPECT().ListMatchHistory(gomock.Any(), int64(1), 10, 0).
					Return(nil, errors.New("db error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := repomocks.NewMockRatingsRepository(ctrl)
			tt.setup(repo)

			svc := service.NewRatingsService(repo, zap.NewNop(), nil)
			got, err := svc.ListMatchHistory(context.Background(), tt.userID, tt.limit, tt.offset)

			if tt.wantErr {
				if err == nil {
					t.Fatal("ListMatchHistory() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("ListMatchHistory() unexpected error = %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("ListMatchHistory() len = %d, want %d", len(got), len(tt.want))
			}

			for i, item := range got {
				w := tt.want[i]
				if item.MatchID != w.MatchID || item.Result != w.Result || item.ELODelta != w.ELODelta {
					t.Errorf("[%d] got %+v, want %+v", i, item, w)
				}
			}
		})
	}
}

func TestRatingsService_GetRating(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		userID  int64
		setup   func(repo *repomocks.MockRatingsRepository)
		want    domain.Rating
		wantErr bool
	}{
		{
			name:   "delegates to repo and returns rating",
			userID: 42,
			setup: func(repo *repomocks.MockRatingsRepository) {
				repo.EXPECT().GetRating(gomock.Any(), int64(42)).
					Return(domain.Rating{UserID: 42, ELO: 1100, Level: 5, GamesPlayed: 25}, nil)
			},
			want: domain.Rating{UserID: 42, ELO: 1100, Level: 5, GamesPlayed: 25},
		},
		{
			name:   "unknown player returns defaults",
			userID: 99,
			setup: func(repo *repomocks.MockRatingsRepository) {
				repo.EXPECT().GetRating(gomock.Any(), int64(99)).
					Return(domain.Rating{
						UserID:      99,
						ELO:         domain.DefaultELO,
						Level:       domain.DefaultLevel,
						GamesPlayed: domain.DefaultGamesPlayed,
					}, nil)
			},
			want: domain.Rating{
				UserID:      99,
				ELO:         domain.DefaultELO,
				Level:       domain.DefaultLevel,
				GamesPlayed: domain.DefaultGamesPlayed,
			},
		},
		{
			name:   "repo error is propagated",
			userID: 7,
			setup: func(repo *repomocks.MockRatingsRepository) {
				repo.EXPECT().GetRating(gomock.Any(), int64(7)).
					Return(domain.Rating{}, errors.New("timeout"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := repomocks.NewMockRatingsRepository(ctrl)
			tt.setup(repo)

			svc := service.NewRatingsService(repo, zap.NewNop(), nil)
			got, err := svc.GetRating(context.Background(), tt.userID)

			if tt.wantErr {
				if err == nil {
					t.Fatal("GetRating() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("GetRating() unexpected error = %v", err)
			}

			if got != tt.want {
				t.Errorf("GetRating() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
