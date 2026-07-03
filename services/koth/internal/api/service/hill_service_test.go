package service_test

import (
	"context"
	"errors"
	"testing"

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

const testWinAmount int64 = 7

// TestHillService_CurrentKing verifies criteria 1 and 5 — CurrentKing parses
// hill_type (400/ErrInvalidHillType on a bad value) and returns
// ErrHillNotFound (404 upstream) when the hill needs seeding.
func TestHillService_CurrentKing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		hillType  string
		setupRepo func(m *repomocks.MockHillRepository)
		wantKing  domain.KingReign
		wantErr   error
	}{
		{
			// criterion: 1 — a valid hill_type returns the current king
			name:     "daily hill returns current king",
			hillType: "daily",
			setupRepo: func(m *repomocks.MockHillRepository) {
				m.EXPECT().CurrentKing(gomock.Any(), domain.HillTypeDaily).
					Return(&domain.KingReign{UserID: 42, ClipID: "clip-1", BlinkTsMs: 8000}, nil)
			},
			wantKing: domain.KingReign{UserID: 42, ClipID: "clip-1", BlinkTsMs: 8000},
		},
		{
			// criterion: 5 — an invalid hill_type is rejected before hitting the repository
			name:      "invalid hill_type returns ErrInvalidHillType",
			hillType:  "weekly",
			setupRepo: func(_ *repomocks.MockHillRepository) {},
			wantErr:   domain.ErrInvalidHillType,
		},
		{
			// criterion: 1 — seeding-404: no current reign surfaces ErrHillNotFound
			name:     "unseeded hill returns ErrHillNotFound",
			hillType: "monthly",
			setupRepo: func(m *repomocks.MockHillRepository) {
				m.EXPECT().CurrentKing(gomock.Any(), domain.HillTypeMonthly).
					Return(nil, repository.ErrHillNotFound)
			},
			wantErr: repository.ErrHillNotFound,
		},
		{
			name:     "repo error propagates",
			hillType: "daily",
			setupRepo: func(m *repomocks.MockHillRepository) {
				m.EXPECT().CurrentKing(gomock.Any(), domain.HillTypeDaily).
					Return(nil, errors.New("db error"))
			},
			wantErr: errors.New("db error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repoMock := repomocks.NewMockHillRepository(ctrl)
			tt.setupRepo(repoMock)

			// No Credit call expected on CurrentKing under any circumstance —
			// no .EXPECT() set on pointsMock means gomock fails the test if
			// Challenge/Credit machinery is ever wired in here by mistake.
			pointsMock := servicemocks.NewMockPointsClient(ctrl)

			svc := service.NewHillService(repoMock, pointsMock, testWinAmount, zap.NewNop())

			got, err := svc.CurrentKing(context.Background(), tt.hillType)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("CurrentKing() error = nil, want %v", tt.wantErr)
				}

				if errors.Is(tt.wantErr, domain.ErrInvalidHillType) && !errors.Is(err, domain.ErrInvalidHillType) {
					t.Errorf("CurrentKing() error = %v, want ErrInvalidHillType", err)
				}

				if errors.Is(tt.wantErr, repository.ErrHillNotFound) && !errors.Is(err, repository.ErrHillNotFound) {
					t.Errorf("CurrentKing() error = %v, want ErrHillNotFound", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("CurrentKing() unexpected error = %v", err)
			}

			if got != tt.wantKing {
				t.Errorf("CurrentKing() = %+v, want %+v", got, tt.wantKing)
			}
		})
	}
}

// TestHillService_Challenge verifies criteria 1, 2, 3, 4, 5, and 6 — Challenge
// parses hill_type, delegates the decide-and-transfer to the repository,
// propagates its outcome (won/lost) and errors unchanged, and — on a crown
// take — credits koth_win points via PointsClient without ever blocking on a
// PointsClient failure.
func TestHillService_Challenge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		hillType    string
		userID      int64
		survivedMs  int
		newClipID   string
		setupRepo   func(m *repomocks.MockHillRepository)
		setupPoints func(m *servicemocks.MockPointsClient)
		wantWon     bool
		wantErr     error
	}{
		{
			// criterion: 1 — a crown take credits PointsClient.Credit with
			// {user_id, reason:"koth_win", ref_id} and the configured win
			// amount, keyed off the reign's own ID.
			name:       "challenger wins takes crown credits koth_win points",
			hillType:   "daily",
			userID:     99,
			survivedMs: 9000,
			newClipID:  "clip-new",
			setupRepo: func(m *repomocks.MockHillRepository) {
				m.EXPECT().Challenge(gomock.Any(), domain.HillTypeDaily, int64(99), 9000, "clip-new").
					Return(domain.ChallengeOutcome{
						Won:  true,
						King: domain.KingReign{ID: 501, UserID: 99, ClipID: "clip-new", BlinkTsMs: 9000},
					}, nil)
			},
			setupPoints: func(m *servicemocks.MockPointsClient) {
				m.EXPECT().Credit(gomock.Any(), service.CreditRequest{
					UserID: 99,
					Reason: "koth_win",
					RefID:  "501",
					Delta:  testWinAmount,
				}).Return(nil)
			},
			wantWon: true,
		},
		{
			// criterion: 2 — a losing challenge (survived_ms < king.blink_ts_ms) reports
			// won=false, the current (unchanged) king, and does NOT credit points.
			name:       "challenger loses king stays no points credited",
			hillType:   "daily",
			userID:     99,
			survivedMs: 5000,
			newClipID:  "clip-new",
			setupRepo: func(m *repomocks.MockHillRepository) {
				m.EXPECT().Challenge(gomock.Any(), domain.HillTypeDaily, int64(99), 5000, "clip-new").
					Return(domain.ChallengeOutcome{
						Won:  false,
						King: domain.KingReign{UserID: 42, ClipID: "clip-1", BlinkTsMs: 8000},
					}, nil)
			},
			setupPoints: func(_ *servicemocks.MockPointsClient) {},
			wantWon:     false,
		},
		{
			// criterion: 4 — a PointsClient failure on a crown take is logged
			// and does NOT block the outcome: Challenge still returns Won=true
			// and nil error even though Credit failed.
			name:       "points client failure on crown take does not block outcome",
			hillType:   "daily",
			userID:     99,
			survivedMs: 9000,
			newClipID:  "clip-new",
			setupRepo: func(m *repomocks.MockHillRepository) {
				m.EXPECT().Challenge(gomock.Any(), domain.HillTypeDaily, int64(99), 9000, "clip-new").
					Return(domain.ChallengeOutcome{
						Won:  true,
						King: domain.KingReign{ID: 502, UserID: 99, ClipID: "clip-new", BlinkTsMs: 9000},
					}, nil)
			},
			setupPoints: func(m *servicemocks.MockPointsClient) {
				m.EXPECT().Credit(gomock.Any(), gomock.Any()).Return(errors.New("store unavailable"))
			},
			wantWon: true,
		},
		{
			// criterion: 5 — an invalid hill_type is rejected before hitting the repository
			name:        "invalid hill_type returns ErrInvalidHillType",
			hillType:    "weekly",
			setupRepo:   func(_ *repomocks.MockHillRepository) {},
			setupPoints: func(_ *servicemocks.MockPointsClient) {},
			wantErr:     domain.ErrInvalidHillType,
		},
		{
			// criterion: 6 — seeding-404: challenging an unseeded hill surfaces ErrHillNotFound
			name:       "unseeded hill returns ErrHillNotFound",
			hillType:   "monthly",
			userID:     99,
			survivedMs: 5000,
			newClipID:  "clip-new",
			setupRepo: func(m *repomocks.MockHillRepository) {
				m.EXPECT().Challenge(gomock.Any(), domain.HillTypeMonthly, int64(99), 5000, "clip-new").
					Return(domain.ChallengeOutcome{}, repository.ErrHillNotFound)
			},
			setupPoints: func(_ *servicemocks.MockPointsClient) {},
			wantErr:     repository.ErrHillNotFound,
		},
		{
			name:       "repo error propagates",
			hillType:   "daily",
			userID:     99,
			survivedMs: 5000,
			newClipID:  "clip-new",
			setupRepo: func(m *repomocks.MockHillRepository) {
				m.EXPECT().Challenge(gomock.Any(), domain.HillTypeDaily, int64(99), 5000, "clip-new").
					Return(domain.ChallengeOutcome{}, errors.New("db error"))
			},
			setupPoints: func(_ *servicemocks.MockPointsClient) {},
			wantErr:     errors.New("db error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repoMock := repomocks.NewMockHillRepository(ctrl)
			tt.setupRepo(repoMock)

			pointsMock := servicemocks.NewMockPointsClient(ctrl)
			tt.setupPoints(pointsMock)

			svc := service.NewHillService(repoMock, pointsMock, testWinAmount, zap.NewNop())

			got, err := svc.Challenge(context.Background(), tt.hillType, tt.userID, tt.survivedMs, tt.newClipID)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("Challenge() error = nil, want %v", tt.wantErr)
				}

				if errors.Is(tt.wantErr, domain.ErrInvalidHillType) && !errors.Is(err, domain.ErrInvalidHillType) {
					t.Errorf("Challenge() error = %v, want ErrInvalidHillType", err)
				}

				if errors.Is(tt.wantErr, repository.ErrHillNotFound) && !errors.Is(err, repository.ErrHillNotFound) {
					t.Errorf("Challenge() error = %v, want ErrHillNotFound", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("Challenge() unexpected error = %v", err)
			}

			if got.Won != tt.wantWon {
				t.Errorf("Challenge() Won = %v, want %v", got.Won, tt.wantWon)
			}
		})
	}
}

// TestHillService_Challenge_IdempotentRefID verifies criterion: 1 — the same
// reign ID (a re-processed identical crown-take event) produces the exact
// same ref_id both times, which is what makes the store's dedup idempotent.
// A broken implementation that derived ref_id from something non-stable
// (e.g. current time) would send two different ref_ids here and fail.
func TestHillService_Challenge_IdempotentRefID(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	repoMock := repomocks.NewMockHillRepository(ctrl)
	pointsMock := servicemocks.NewMockPointsClient(ctrl)

	outcome := domain.ChallengeOutcome{
		Won:  true,
		King: domain.KingReign{ID: 777, UserID: 100, ClipID: "clip-repeat", BlinkTsMs: 9500},
	}

	repoMock.EXPECT().Challenge(gomock.Any(), domain.HillTypeDaily, int64(100), 9500, "clip-repeat").
		Return(outcome, nil).Times(2)

	wantCredit := service.CreditRequest{
		UserID: 100,
		Reason: "koth_win",
		RefID:  "777",
		Delta:  testWinAmount,
	}
	pointsMock.EXPECT().Credit(gomock.Any(), wantCredit).Return(nil).Times(2)

	svc := service.NewHillService(repoMock, pointsMock, testWinAmount, zap.NewNop())

	for i := range 2 {
		got, err := svc.Challenge(context.Background(), "daily", 100, 9500, "clip-repeat")
		if err != nil {
			t.Fatalf("Challenge() call %d unexpected error = %v", i, err)
		}

		if !got.Won {
			t.Fatalf("Challenge() call %d Won = false, want true", i)
		}
	}
}

// TestHillService_Challenge_LogsPointsClientFailure verifies criterion: 4 —
// a PointsClient failure on a crown take is actually logged (zap, error
// level), not merely swallowed silently.
func TestHillService_Challenge_LogsPointsClientFailure(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	repoMock := repomocks.NewMockHillRepository(ctrl)
	repoMock.EXPECT().Challenge(gomock.Any(), domain.HillTypeDaily, int64(99), 9000, "clip-new").
		Return(domain.ChallengeOutcome{
			Won:  true,
			King: domain.KingReign{ID: 502, UserID: 99, ClipID: "clip-new", BlinkTsMs: 9000},
		}, nil)

	pointsMock := servicemocks.NewMockPointsClient(ctrl)
	pointsMock.EXPECT().Credit(gomock.Any(), gomock.Any()).Return(errors.New("store unavailable"))

	core, logs := observer.New(zapcore.ErrorLevel)
	logger := zap.New(core)

	svc := service.NewHillService(repoMock, pointsMock, testWinAmount, logger)

	got, err := svc.Challenge(context.Background(), "daily", 99, 9000, "clip-new")
	if err != nil {
		t.Fatalf("Challenge() unexpected error = %v", err)
	}

	if !got.Won {
		t.Fatalf("Challenge() Won = false, want true")
	}

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("len(logged entries) = %d, want 1", len(entries))
	}

	if entries[0].Level != zapcore.ErrorLevel {
		t.Errorf("logged entry level = %v, want %v", entries[0].Level, zapcore.ErrorLevel)
	}
}
