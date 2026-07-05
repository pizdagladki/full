package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/pizdagladki/full/services/koth/internal/api/domain"
	repomocks "github.com/pizdagladki/full/services/koth/internal/api/repository/mocks"
	"github.com/pizdagladki/full/services/koth/internal/api/service"
	svcmocks "github.com/pizdagladki/full/services/koth/internal/api/service/mocks"
)

// TestResetService_CloseStaleReign is table-driven and covers: daily
// close+clear, monthly close, the clip-expiry call, idempotent re-run
// (nothing to close), the points-client-failure-non-blocking path, and the
// empty-ClipID skip.
func TestResetService_CloseStaleReign(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 4, 3, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: now}

	dailyPeriodStart := domain.PeriodStart(domain.HillTypeDaily, now)
	monthlyPeriodStart := domain.PeriodStart(domain.HillTypeMonthly, now)

	tests := []struct {
		name       string
		hillType   domain.HillType
		setupRepo  func(m *repomocks.MockHillRepository)
		setupPts   func(m *svcmocks.MockPointsClient)
		setupMedia func(m *svcmocks.MockMediaClient)
		wantErr    bool
	}{
		{
			// criterion: 1 — a stale DAILY reign is closed, the final-placement reward
			// is credited with ReasonKothDailyFinal, and the king clip is expired.
			name:     "daily close credits reward and clears king",
			hillType: domain.HillTypeDaily,
			setupRepo: func(m *repomocks.MockHillRepository) {
				m.EXPECT().CloseIfStale(gomock.Any(), domain.HillTypeDaily, dailyPeriodStart).
					Return(&domain.KingReign{
						ID: 1, HillType: domain.HillTypeDaily, UserID: 42, ClipID: "clip-1",
						BlinkTsMs: 8000, StartedAt: dailyPeriodStart.Add(-time.Hour),
					}, nil)
			},
			setupPts: func(m *svcmocks.MockPointsClient) {
				refID := "daily:" + dailyPeriodStart.Format("2006-01-02")
				m.EXPECT().Credit(gomock.Any(), service.CreditRequest{
					UserID: 42, Reason: domain.ReasonKothDailyFinal, RefID: refID,
				}).Return(nil)
			},
			setupMedia: func(m *svcmocks.MockMediaClient) {
				// criterion: 3 — the closed reign's clip is expired via the media client.
				m.EXPECT().ExpireKingClip(gomock.Any(), "clip-1").Return(nil)
			},
		},
		{
			// criterion: 2 — a stale MONTHLY reign is closed with the bigger
			// final-placement reward reason (ReasonKothMonthlyFinal).
			name:     "monthly close credits bigger reward",
			hillType: domain.HillTypeMonthly,
			setupRepo: func(m *repomocks.MockHillRepository) {
				m.EXPECT().CloseIfStale(gomock.Any(), domain.HillTypeMonthly, monthlyPeriodStart).
					Return(&domain.KingReign{
						ID: 2, HillType: domain.HillTypeMonthly, UserID: 7, ClipID: "clip-mo",
						BlinkTsMs: 20000, StartedAt: monthlyPeriodStart.Add(-time.Hour),
					}, nil)
			},
			setupPts: func(m *svcmocks.MockPointsClient) {
				refID := "monthly:" + monthlyPeriodStart.Format("2006-01-02")
				m.EXPECT().Credit(gomock.Any(), service.CreditRequest{
					UserID: 7, Reason: domain.ReasonKothMonthlyFinal, RefID: refID,
				}).Return(nil)
			},
			setupMedia: func(m *svcmocks.MockMediaClient) {
				m.EXPECT().ExpireKingClip(gomock.Any(), "clip-mo").Return(nil)
			},
		},
		{
			// criterion: 4 — a second run for the same period is a no-op: the
			// repository reports nothing to close, and NEITHER client is called —
			// this is what makes the job idempotent-per-period.
			name:     "idempotent re-run calls neither client",
			hillType: domain.HillTypeDaily,
			setupRepo: func(m *repomocks.MockHillRepository) {
				m.EXPECT().CloseIfStale(gomock.Any(), domain.HillTypeDaily, dailyPeriodStart).
					Return(nil, nil)
			},
			setupPts:   func(_ *svcmocks.MockPointsClient) {},
			setupMedia: func(_ *svcmocks.MockMediaClient) {},
		},
		{
			// criterion: 4 (non-blocking) — a PointsClient failure is logged and does
			// NOT block the clip expiry or the overall (nil) result.
			name:     "points client failure does not block clip expiry",
			hillType: domain.HillTypeDaily,
			setupRepo: func(m *repomocks.MockHillRepository) {
				m.EXPECT().CloseIfStale(gomock.Any(), domain.HillTypeDaily, dailyPeriodStart).
					Return(&domain.KingReign{
						ID: 3, HillType: domain.HillTypeDaily, UserID: 99, ClipID: "clip-3",
						BlinkTsMs: 5000, StartedAt: dailyPeriodStart.Add(-time.Hour),
					}, nil)
			},
			setupPts: func(m *svcmocks.MockPointsClient) {
				m.EXPECT().Credit(gomock.Any(), gomock.Any()).Return(errors.New("store unreachable"))
			},
			setupMedia: func(m *svcmocks.MockMediaClient) {
				m.EXPECT().ExpireKingClip(gomock.Any(), "clip-3").Return(nil)
			},
		},
		{
			// criterion: closing a reign with no clip (empty ClipID) does NOT call
			// the media client at all.
			name:     "empty clip id skips media client",
			hillType: domain.HillTypeDaily,
			setupRepo: func(m *repomocks.MockHillRepository) {
				m.EXPECT().CloseIfStale(gomock.Any(), domain.HillTypeDaily, dailyPeriodStart).
					Return(&domain.KingReign{
						ID: 4, HillType: domain.HillTypeDaily, UserID: 11, ClipID: "",
						BlinkTsMs: 4000, StartedAt: dailyPeriodStart.Add(-time.Hour),
					}, nil)
			},
			setupPts: func(m *svcmocks.MockPointsClient) {
				m.EXPECT().Credit(gomock.Any(), gomock.Any()).Return(nil)
			},
			setupMedia: func(_ *svcmocks.MockMediaClient) {},
		},
		{
			name:     "repo error propagates",
			hillType: domain.HillTypeDaily,
			setupRepo: func(m *repomocks.MockHillRepository) {
				m.EXPECT().CloseIfStale(gomock.Any(), domain.HillTypeDaily, dailyPeriodStart).
					Return(nil, errors.New("db error"))
			},
			setupPts:   func(_ *svcmocks.MockPointsClient) {},
			setupMedia: func(_ *svcmocks.MockMediaClient) {},
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repoMock := repomocks.NewMockHillRepository(ctrl)
			ptsMock := svcmocks.NewMockPointsClient(ctrl)
			mediaMock := svcmocks.NewMockMediaClient(ctrl)

			tt.setupRepo(repoMock)
			tt.setupPts(ptsMock)
			tt.setupMedia(mediaMock)

			svc := service.NewResetService(repoMock, clock, ptsMock, mediaMock, zap.NewNop())

			err := svc.CloseStaleReign(context.Background(), tt.hillType)

			if tt.wantErr {
				if err == nil {
					t.Fatal("CloseStaleReign() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("CloseStaleReign() unexpected error = %v", err)
			}
		})
	}
}

// TestResetService_CloseStaleReign_LogsCreditFailure verifies criterion:
// a failed PointsClient.Credit is logged as a warning rather than silently
// swallowed, while CloseStaleReign still returns nil.
func TestResetService_CloseStaleReign_LogsCreditFailure(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zap.WarnLevel)
	logger := zap.New(core)

	now := time.Date(2026, 7, 4, 3, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: now}
	dailyPeriodStart := domain.PeriodStart(domain.HillTypeDaily, now)

	ctrl := gomock.NewController(t)
	repoMock := repomocks.NewMockHillRepository(ctrl)
	ptsMock := svcmocks.NewMockPointsClient(ctrl)
	mediaMock := svcmocks.NewMockMediaClient(ctrl)

	repoMock.EXPECT().CloseIfStale(gomock.Any(), domain.HillTypeDaily, dailyPeriodStart).
		Return(&domain.KingReign{
			ID: 1, HillType: domain.HillTypeDaily, UserID: 1, ClipID: "clip-x",
			BlinkTsMs: 1000, StartedAt: dailyPeriodStart.Add(-time.Hour),
		}, nil)
	ptsMock.EXPECT().Credit(gomock.Any(), gomock.Any()).Return(errors.New("boom"))
	mediaMock.EXPECT().ExpireKingClip(gomock.Any(), "clip-x").Return(nil)

	svc := service.NewResetService(repoMock, clock, ptsMock, mediaMock, logger)

	err := svc.CloseStaleReign(context.Background(), domain.HillTypeDaily)
	if err != nil {
		t.Fatalf("CloseStaleReign() unexpected error = %v", err)
	}

	if logs.Len() == 0 {
		t.Fatal("expected a warning log entry for the failed credit, got none")
	}
}
