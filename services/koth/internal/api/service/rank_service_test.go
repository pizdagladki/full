package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/pizdagladki/full/services/koth/internal/api/domain"
	repomocks "github.com/pizdagladki/full/services/koth/internal/api/repository/mocks"
	"github.com/pizdagladki/full/services/koth/internal/api/service"

	"github.com/pizdagladki/full/services/koth/internal/api/repository"
)

// fakeClock is an injectable, controllable time source for the daily-reset
// boundary tests.
type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time { return c.now }

var testThresholds = []int{5000, 15000, 30000, 60000, 120000}

// TestRankService_SubmitAttempt verifies criterion: 1 — SubmitAttempt
// computes the achieved rank from the thresholds and, only when it exceeds
// the stored rank, upserts the new rank + best_hold_ms; a worse attempt never
// downgrades the stored rank.
func TestRankService_SubmitAttempt(t *testing.T) {
	t.Parallel()

	day := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		userID       int64
		heldMs       int
		setupRepo    func(m *repomocks.MockRankRepository)
		wantErr      error
		wantAchieved int
		wantCurrent  int
		wantNew      bool
	}{
		{
			// criterion: 5 — a non-positive held_ms is rejected with ErrInvalidHoldMs
			name:      "zero held ms returns ErrInvalidHoldMs",
			userID:    1,
			heldMs:    0,
			setupRepo: func(_ *repomocks.MockRankRepository) {},
			wantErr:   domain.ErrInvalidHoldMs,
		},
		{
			// criterion: 5 — a negative held_ms is rejected with ErrInvalidHoldMs
			name:      "negative held ms returns ErrInvalidHoldMs",
			userID:    1,
			heldMs:    -100,
			setupRepo: func(_ *repomocks.MockRankRepository) {},
			wantErr:   domain.ErrInvalidHoldMs,
		},
		{
			// criterion: 1 — first attempt with no existing row records the new rank
			name:   "first attempt with no existing row records new rank",
			userID: 1,
			heldMs: 16000,
			setupRepo: func(m *repomocks.MockRankRepository) {
				m.EXPECT().GetRank(gomock.Any(), int64(1), day).
					Return(nil, repository.ErrRankNotFound)
				m.EXPECT().UpsertRank(gomock.Any(), int64(1), day, 2, 16000).
					Return(nil)
			},
			wantAchieved: 2,
			wantCurrent:  2,
			wantNew:      true,
		},
		{
			// criterion: 1 — an attempt that beats the current rank upserts the new rank
			name:   "better attempt upserts new rank",
			userID: 2,
			heldMs: 31000,
			setupRepo: func(m *repomocks.MockRankRepository) {
				m.EXPECT().GetRank(gomock.Any(), int64(2), day).
					Return(&domain.HillRank{UserID: 2, Day: day, Rank: 2, BestHoldMs: 16000}, nil)
				m.EXPECT().UpsertRank(gomock.Any(), int64(2), day, 3, 31000).
					Return(nil)
			},
			wantAchieved: 3,
			wantCurrent:  3,
			wantNew:      true,
		},
		{
			// criterion: 1 — a worse attempt (lower achieved rank) does NOT downgrade the stored rank
			name:   "worse attempt does not downgrade stored rank",
			userID: 3,
			heldMs: 6000,
			setupRepo: func(m *repomocks.MockRankRepository) {
				m.EXPECT().GetRank(gomock.Any(), int64(3), day).
					Return(&domain.HillRank{UserID: 3, Day: day, Rank: 3, BestHoldMs: 31000}, nil)
				// UpsertRank must NOT be called.
			},
			wantAchieved: 1,
			wantCurrent:  3,
			wantNew:      false,
		},
		{
			// criterion: 1 — an equal attempt (same achieved rank) does not re-trigger newly_reached
			name:   "equal attempt does not re-trigger newly reached",
			userID: 4,
			heldMs: 16000,
			setupRepo: func(m *repomocks.MockRankRepository) {
				m.EXPECT().GetRank(gomock.Any(), int64(4), day).
					Return(&domain.HillRank{UserID: 4, Day: day, Rank: 2, BestHoldMs: 20000}, nil)
			},
			wantAchieved: 2,
			wantCurrent:  2,
			wantNew:      false,
		},
		{
			name:   "get rank repo error propagates",
			userID: 5,
			heldMs: 6000,
			setupRepo: func(m *repomocks.MockRankRepository) {
				m.EXPECT().GetRank(gomock.Any(), int64(5), day).
					Return(nil, errors.New("db error"))
			},
			wantErr: errors.New("db error"),
		},
		{
			name:   "upsert repo error propagates",
			userID: 6,
			heldMs: 6000,
			setupRepo: func(m *repomocks.MockRankRepository) {
				m.EXPECT().GetRank(gomock.Any(), int64(6), day).
					Return(nil, repository.ErrRankNotFound)
				m.EXPECT().UpsertRank(gomock.Any(), int64(6), day, 1, 6000).
					Return(errors.New("db error"))
			},
			wantErr: errors.New("db error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repoMock := repomocks.NewMockRankRepository(ctrl)
			tt.setupRepo(repoMock)

			svc := service.NewRankService(repoMock, &fakeClock{now: day}, testThresholds)

			got, err := svc.SubmitAttempt(context.Background(), tt.userID, tt.heldMs)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("SubmitAttempt() error = nil, want %v", tt.wantErr)
				}

				if errors.Is(tt.wantErr, domain.ErrInvalidHoldMs) && !errors.Is(err, domain.ErrInvalidHoldMs) {
					t.Errorf("SubmitAttempt() error = %v, want ErrInvalidHoldMs", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("SubmitAttempt() unexpected error = %v", err)
			}

			if got.AchievedRank != tt.wantAchieved {
				t.Errorf("AchievedRank = %d, want %d", got.AchievedRank, tt.wantAchieved)
			}

			if got.CurrentRank != tt.wantCurrent {
				t.Errorf("CurrentRank = %d, want %d", got.CurrentRank, tt.wantCurrent)
			}

			if got.NewlyReached != tt.wantNew {
				t.Errorf("NewlyReached = %v, want %v", got.NewlyReached, tt.wantNew)
			}
		})
	}
}

// TestRankService_Me verifies criterion: 2 — Me returns {current_rank,
// next_target_ms} for today, defaulting to rank 0 with the rank-1 threshold
// when the player has no row today.
func TestRankService_Me(t *testing.T) {
	t.Parallel()

	day := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		userID      int64
		setupRepo   func(m *repomocks.MockRankRepository)
		wantCurrent int
		wantTarget  int
		wantErr     bool
	}{
		{
			// criterion: 2 — no row today defaults to rank 0 with the rank-1 threshold
			name:   "no row today defaults to rank 0",
			userID: 1,
			setupRepo: func(m *repomocks.MockRankRepository) {
				m.EXPECT().GetRank(gomock.Any(), int64(1), day).
					Return(nil, repository.ErrRankNotFound)
			},
			wantCurrent: 0,
			wantTarget:  5000,
		},
		{
			// criterion: 2 — an existing row reports the stored rank and next target
			name:   "existing row reports stored rank and next target",
			userID: 2,
			setupRepo: func(m *repomocks.MockRankRepository) {
				m.EXPECT().GetRank(gomock.Any(), int64(2), day).
					Return(&domain.HillRank{UserID: 2, Day: day, Rank: 2, BestHoldMs: 16000}, nil)
			},
			wantCurrent: 2,
			wantTarget:  30000,
		},
		{
			// criterion: 2 — max rank reports no higher target (0)
			name:   "max rank reports no higher target",
			userID: 3,
			setupRepo: func(m *repomocks.MockRankRepository) {
				m.EXPECT().GetRank(gomock.Any(), int64(3), day).
					Return(&domain.HillRank{UserID: 3, Day: day, Rank: 5, BestHoldMs: 200000}, nil)
			},
			wantCurrent: 5,
			wantTarget:  0,
		},
		{
			name:   "repo error propagates",
			userID: 4,
			setupRepo: func(m *repomocks.MockRankRepository) {
				m.EXPECT().GetRank(gomock.Any(), int64(4), day).
					Return(nil, errors.New("db error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repoMock := repomocks.NewMockRankRepository(ctrl)
			tt.setupRepo(repoMock)

			svc := service.NewRankService(repoMock, &fakeClock{now: day}, testThresholds)

			got, err := svc.Me(context.Background(), tt.userID)

			if tt.wantErr {
				if err == nil {
					t.Fatal("Me() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("Me() unexpected error = %v", err)
			}

			if got.CurrentRank != tt.wantCurrent {
				t.Errorf("CurrentRank = %d, want %d", got.CurrentRank, tt.wantCurrent)
			}

			if got.NextTargetMs != tt.wantTarget {
				t.Errorf("NextTargetMs = %d, want %d", got.NextTargetMs, tt.wantTarget)
			}
		})
	}
}

// TestRankService_Leaderboard verifies criterion: 3 — Leaderboard returns the
// accounts-per-rank distribution for today.
func TestRankService_Leaderboard(t *testing.T) {
	t.Parallel()

	day := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		setupRepo func(m *repomocks.MockRankRepository)
		want      []domain.RankCount
		wantErr   bool
	}{
		{
			// criterion: 3 — returns the distribution of accounts across ranks for today
			name: "returns distribution for today",
			setupRepo: func(m *repomocks.MockRankRepository) {
				m.EXPECT().RankDistribution(gomock.Any(), day).
					Return([]domain.RankCount{{Rank: 0, Count: 3}, {Rank: 1, Count: 5}}, nil)
			},
			want: []domain.RankCount{{Rank: 0, Count: 3}, {Rank: 1, Count: 5}},
		},
		{
			name: "repo error propagates",
			setupRepo: func(m *repomocks.MockRankRepository) {
				m.EXPECT().RankDistribution(gomock.Any(), day).
					Return(nil, errors.New("db error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repoMock := repomocks.NewMockRankRepository(ctrl)
			tt.setupRepo(repoMock)

			svc := service.NewRankService(repoMock, &fakeClock{now: day}, testThresholds)

			got, err := svc.Leaderboard(context.Background())

			if tt.wantErr {
				if err == nil {
					t.Fatal("Leaderboard() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("Leaderboard() unexpected error = %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("len(distribution) = %d, want %d", len(got), len(tt.want))
			}

			for i, rc := range got {
				if rc != tt.want[i] {
					t.Errorf("distribution[%d] = %+v, want %+v", i, rc, tt.want[i])
				}
			}
		})
	}
}

// TestRankService_DailyReset verifies criterion: 4 — ranks are per-day: a new
// day starts everyone at rank 0 and a query for a prior day does not leak
// into today. Uses a fake clock to control which day requests land on.
func TestRankService_DailyReset(t *testing.T) {
	t.Parallel()

	dayOne := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)
	dayTwo := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)

	clock := &fakeClock{now: dayOne}

	ctrl := gomock.NewController(t)
	repoMock := repomocks.NewMockRankRepository(ctrl)

	// Day one: player reaches rank 3.
	repoMock.EXPECT().GetRank(gomock.Any(), int64(1), dayOne).
		Return(nil, repository.ErrRankNotFound)
	repoMock.EXPECT().UpsertRank(gomock.Any(), int64(1), dayOne, 3, 31000).
		Return(nil)

	svc := service.NewRankService(repoMock, clock, testThresholds)

	got, err := svc.SubmitAttempt(context.Background(), 1, 31000)
	if err != nil {
		t.Fatalf("SubmitAttempt() day one unexpected error = %v", err)
	}

	if got.CurrentRank != 3 {
		t.Fatalf("day one CurrentRank = %d, want 3", got.CurrentRank)
	}

	// Advance the fake clock to a new day: no row yet for dayTwo, so the
	// player starts back at rank 0 — the prior day's rank must not leak in.
	clock.now = dayTwo

	repoMock.EXPECT().GetRank(gomock.Any(), int64(1), dayTwo).
		Return(nil, repository.ErrRankNotFound)

	meDayTwo, err := svc.Me(context.Background(), 1)
	if err != nil {
		t.Fatalf("Me() day two unexpected error = %v", err)
	}

	if meDayTwo.CurrentRank != 0 {
		t.Errorf("day two CurrentRank = %d, want 0 (daily reset, no leak from day one)", meDayTwo.CurrentRank)
	}
}
