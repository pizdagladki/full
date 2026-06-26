package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/ratings/internal/api/domain"
	repomocks "github.com/pizdagladki/full/services/ratings/internal/api/repository/mocks"
)

func TestRatingsService_ApplyMatchResult(t *testing.T) {
	t.Parallel()

	dur := 3000

	tests := []struct {
		name    string
		input   domain.MatchInput
		setup   func(repo *repomocks.MockRatingsRepository)
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
				}, nil)
			},
			want: domain.MatchResult{
				Winner:      domain.Rating{UserID: 1, ELO: 1016, Level: 4, GamesPlayed: 1},
				Loser:       domain.Rating{UserID: 2, ELO: 987, Level: 4, GamesPlayed: 1},
				WinnerDelta: 16,
				LoserDelta:  -13,
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
			wantErr: ErrSamePlayer,
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := repomocks.NewMockRatingsRepository(ctrl)
			tt.setup(repo)

			svc := NewRatingsService(repo, zap.NewNop())
			got, err := svc.ApplyMatchResult(context.Background(), tt.input)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("ApplyMatchResult() error = nil, want %v", tt.wantErr)
				}
				// For sentinel errors check exact match; for wrapped errors check wrapping.
				if errors.Is(tt.wantErr, ErrSamePlayer) && !errors.Is(err, ErrSamePlayer) {
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

			svc := NewRatingsService(repo, zap.NewNop())
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

			svc := NewRatingsService(repo, zap.NewNop())
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
