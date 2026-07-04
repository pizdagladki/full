package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/pizdagladki/full/services/koth/internal/api/domain"
	repomocks "github.com/pizdagladki/full/services/koth/internal/api/repository/mocks"
	"github.com/pizdagladki/full/services/koth/internal/api/service"
	servicemocks "github.com/pizdagladki/full/services/koth/internal/api/service/mocks"

	"github.com/pizdagladki/full/services/koth/internal/api/repository"
)

// fakeClock is an injectable, controllable time source for the daily-reset
// boundary tests.
type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time { return c.now }

var testThresholds = []int{5000, 15000, 30000, 60000, 120000}

const testRankAmount int64 = 4

// noPointsCall returns a MockPointsClient with no .EXPECT() set — gomock
// fails the test if Credit is ever called, asserting "no credit" for the
// non-newly-reached / error paths.
func noPointsCall(ctrl *gomock.Controller) *servicemocks.MockPointsClient {
	return servicemocks.NewMockPointsClient(ctrl)
}

// TestRankService_SubmitAttempt verifies criteria 1 and 5 — SubmitAttempt
// computes the achieved rank from the thresholds and, only when it exceeds
// the stored rank, upserts the new rank + best_hold_ms (a worse attempt never
// downgrades the stored rank), crediting koth_rank points via PointsClient
// only on a newly-reached rank, and never blocking on a PointsClient failure.
func TestRankService_SubmitAttempt(t *testing.T) {
	t.Parallel()

	day := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		userID       int64
		heldMs       int
		setupRepo    func(m *repomocks.MockRankRepository)
		setupPoints  func(m *servicemocks.MockPointsClient)
		wantErr      error
		wantAchieved int
		wantCurrent  int
		wantNew      bool
	}{
		{
			// criterion: 5 — a non-positive held_ms is rejected with ErrInvalidHoldMs
			name:        "zero held ms returns ErrInvalidHoldMs",
			userID:      1,
			heldMs:      0,
			setupRepo:   func(_ *repomocks.MockRankRepository) {},
			setupPoints: func(_ *servicemocks.MockPointsClient) {},
			wantErr:     domain.ErrInvalidHoldMs,
		},
		{
			// criterion: 5 — a negative held_ms is rejected with ErrInvalidHoldMs
			name:        "negative held ms returns ErrInvalidHoldMs",
			userID:      1,
			heldMs:      -100,
			setupRepo:   func(_ *repomocks.MockRankRepository) {},
			setupPoints: func(_ *servicemocks.MockPointsClient) {},
			wantErr:     domain.ErrInvalidHoldMs,
		},
		{
			// criterion: 1 — first attempt with no existing row records the new
			// rank AND credits PointsClient.Credit with
			// {user_id, reason:"koth_rank", ref_id} keyed off day+achieved-rank.
			name:   "first attempt with no existing row records new rank and credits koth_rank",
			userID: 1,
			heldMs: 16000,
			setupRepo: func(m *repomocks.MockRankRepository) {
				m.EXPECT().GetRank(gomock.Any(), int64(1), day).
					Return(nil, repository.ErrRankNotFound)
				m.EXPECT().UpsertRank(gomock.Any(), int64(1), day, 2, 16000).
					Return(nil)
			},
			setupPoints: func(m *servicemocks.MockPointsClient) {
				m.EXPECT().Credit(gomock.Any(), service.CreditRequest{
					UserID: 1,
					Reason: "koth_rank",
					RefID:  "2026-07-03:2",
					Delta:  testRankAmount,
				}).Return(nil)
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
			setupPoints: func(m *servicemocks.MockPointsClient) {
				m.EXPECT().Credit(gomock.Any(), service.CreditRequest{
					UserID: 2,
					Reason: "koth_rank",
					RefID:  "2026-07-03:3",
					Delta:  testRankAmount,
				}).Return(nil)
			},
			wantAchieved: 3,
			wantCurrent:  3,
			wantNew:      true,
		},
		{
			// criterion: 4 — a PointsClient failure on a rank-up is logged and
			// does NOT block the outcome: SubmitAttempt still reports
			// NewlyReached=true and nil error even though Credit failed.
			name:   "points client failure on rank-up does not block outcome",
			userID: 8,
			heldMs: 16000,
			setupRepo: func(m *repomocks.MockRankRepository) {
				m.EXPECT().GetRank(gomock.Any(), int64(8), day).
					Return(nil, repository.ErrRankNotFound)
				m.EXPECT().UpsertRank(gomock.Any(), int64(8), day, 2, 16000).
					Return(nil)
			},
			setupPoints: func(m *servicemocks.MockPointsClient) {
				m.EXPECT().Credit(gomock.Any(), gomock.Any()).Return(errors.New("store unavailable"))
			},
			wantAchieved: 2,
			wantCurrent:  2,
			wantNew:      true,
		},
		{
			// criterion: 1 — a worse attempt (lower achieved rank) does NOT
			// downgrade the stored rank and does NOT credit points.
			name:   "worse attempt does not downgrade stored rank or credit points",
			userID: 3,
			heldMs: 6000,
			setupRepo: func(m *repomocks.MockRankRepository) {
				m.EXPECT().GetRank(gomock.Any(), int64(3), day).
					Return(&domain.HillRank{UserID: 3, Day: day, Rank: 3, BestHoldMs: 31000}, nil)
				// UpsertRank must NOT be called.
			},
			setupPoints:  func(_ *servicemocks.MockPointsClient) {},
			wantAchieved: 1,
			wantCurrent:  3,
			wantNew:      false,
		},
		{
			// criterion: 1 — an equal attempt (same achieved rank) does not
			// re-trigger newly_reached and does not credit points again.
			name:   "equal attempt does not re-trigger newly reached or credit points",
			userID: 4,
			heldMs: 16000,
			setupRepo: func(m *repomocks.MockRankRepository) {
				m.EXPECT().GetRank(gomock.Any(), int64(4), day).
					Return(&domain.HillRank{UserID: 4, Day: day, Rank: 2, BestHoldMs: 20000}, nil)
			},
			setupPoints:  func(_ *servicemocks.MockPointsClient) {},
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
			setupPoints: func(_ *servicemocks.MockPointsClient) {},
			wantErr:     errors.New("db error"),
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
			setupPoints: func(_ *servicemocks.MockPointsClient) {},
			wantErr:     errors.New("db error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repoMock := repomocks.NewMockRankRepository(ctrl)
			tt.setupRepo(repoMock)

			pointsMock := noPointsCall(ctrl)
			tt.setupPoints(pointsMock)

			svc := service.NewRankService(repoMock, &fakeClock{now: day}, testThresholds, pointsMock, testRankAmount, zap.NewNop())

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

// TestRankService_SubmitAttempt_IdempotentRefID verifies criterion: 1 — a
// repeated identical rank-up event (same day, same achieved rank) produces
// the exact same ref_id both times, which is what makes the store's dedup
// idempotent. A broken implementation that derived ref_id from something
// non-stable (e.g. current time or a random ID) would send two different
// ref_ids here and fail.
func TestRankService_SubmitAttempt_IdempotentRefID(t *testing.T) {
	t.Parallel()

	day := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)

	ctrl := gomock.NewController(t)
	repoMock := repomocks.NewMockRankRepository(ctrl)
	pointsMock := servicemocks.NewMockPointsClient(ctrl)

	repoMock.EXPECT().GetRank(gomock.Any(), int64(7), day).
		Return(nil, repository.ErrRankNotFound).Times(2)
	repoMock.EXPECT().UpsertRank(gomock.Any(), int64(7), day, 2, 16000).
		Return(nil).Times(2)

	wantCredit := service.CreditRequest{
		UserID: 7,
		Reason: "koth_rank",
		RefID:  "2026-07-03:2",
		Delta:  testRankAmount,
	}
	pointsMock.EXPECT().Credit(gomock.Any(), wantCredit).Return(nil).Times(2)

	svc := service.NewRankService(repoMock, &fakeClock{now: day}, testThresholds, pointsMock, testRankAmount, zap.NewNop())

	for i := range 2 {
		got, err := svc.SubmitAttempt(context.Background(), 7, 16000)
		if err != nil {
			t.Fatalf("SubmitAttempt() call %d unexpected error = %v", i, err)
		}

		if !got.NewlyReached {
			t.Fatalf("SubmitAttempt() call %d NewlyReached = false, want true", i)
		}
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

			pointsMock := noPointsCall(ctrl)

			svc := service.NewRankService(repoMock, &fakeClock{now: day}, testThresholds, pointsMock, testRankAmount, zap.NewNop())

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

			pointsMock := noPointsCall(ctrl)

			svc := service.NewRankService(repoMock, &fakeClock{now: day}, testThresholds, pointsMock, testRankAmount, zap.NewNop())

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
	pointsMock := servicemocks.NewMockPointsClient(ctrl)

	// Day one: player reaches rank 3, crediting koth_rank points keyed to day one.
	repoMock.EXPECT().GetRank(gomock.Any(), int64(1), dayOne).
		Return(nil, repository.ErrRankNotFound)
	repoMock.EXPECT().UpsertRank(gomock.Any(), int64(1), dayOne, 3, 31000).
		Return(nil)
	pointsMock.EXPECT().Credit(gomock.Any(), service.CreditRequest{
		UserID: 1,
		Reason: "koth_rank",
		RefID:  "2026-07-02:3",
		Delta:  testRankAmount,
	}).Return(nil)

	svc := service.NewRankService(repoMock, clock, testThresholds, pointsMock, testRankAmount, zap.NewNop())

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

// TestRankService_SubmitAttempt_LogsPointsClientFailure verifies criterion: 4
// — a PointsClient failure on a rank-up is actually logged (zap, error
// level), not merely swallowed silently.
func TestRankService_SubmitAttempt_LogsPointsClientFailure(t *testing.T) {
	t.Parallel()

	day := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)

	ctrl := gomock.NewController(t)
	repoMock := repomocks.NewMockRankRepository(ctrl)
	repoMock.EXPECT().GetRank(gomock.Any(), int64(8), day).
		Return(nil, repository.ErrRankNotFound)
	repoMock.EXPECT().UpsertRank(gomock.Any(), int64(8), day, 2, 16000).
		Return(nil)

	pointsMock := servicemocks.NewMockPointsClient(ctrl)
	pointsMock.EXPECT().Credit(gomock.Any(), gomock.Any()).Return(errors.New("store unavailable"))

	core, logs := observer.New(zapcore.ErrorLevel)
	logger := zap.New(core)

	svc := service.NewRankService(repoMock, &fakeClock{now: day}, testThresholds, pointsMock, testRankAmount, logger)

	got, err := svc.SubmitAttempt(context.Background(), 8, 16000)
	if err != nil {
		t.Fatalf("SubmitAttempt() unexpected error = %v", err)
	}

	if !got.NewlyReached {
		t.Fatalf("SubmitAttempt() NewlyReached = false, want true")
	}

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("len(logged entries) = %d, want 1", len(entries))
	}

	if entries[0].Level != zapcore.ErrorLevel {
		t.Errorf("logged entry level = %v, want %v", entries[0].Level, zapcore.ErrorLevel)
	}
}
