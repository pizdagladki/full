package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/reports/internal/api/domain"
	repomocks "github.com/pizdagladki/full/services/reports/internal/api/repository/mocks"
	"github.com/pizdagladki/full/services/reports/internal/api/service"
)

const (
	testCooldownTTL = 30 * time.Minute
	reporterID      = int64(1)
	reportedID      = int64(2)
	testMatchID     = "match-abc"
)

func newService(t *testing.T, repo *repomocks.MockCheatReportsRepository, cooldown *repomocks.MockCooldownStore) service.ReportsService {
	t.Helper()

	return service.NewReportsService(repo, cooldown, zap.NewNop(), testCooldownTTL)
}

func TestReportCheat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		reporterID int64
		reportedID int64
		matchID    string
		setupMock  func(repo *repomocks.MockCheatReportsRepository, cooldown *repomocks.MockCooldownStore)
		wantErr    error
	}{
		{
			// criterion: 1 — insert succeeds, count=4, no cooldown
			name:       "insert and no cooldown when count below threshold",
			reporterID: reporterID,
			reportedID: reportedID,
			matchID:    testMatchID,
			setupMock: func(repo *repomocks.MockCheatReportsRepository, cooldown *repomocks.MockCooldownStore) {
				repo.EXPECT().
					UpsertCheatReport(gomock.Any(), domain.CheatReport{ReporterID: reporterID, ReportedID: reportedID, MatchID: testMatchID}).
					Return(nil)
				repo.EXPECT().
					CountRecentCheatReports(gomock.Any(), reportedID, 10).
					Return(4, nil)
				// SetCooldown must NOT be called
			},
		},
		{
			// criterion: 1 — idempotent re-report (ON CONFLICT DO NOTHING returns nil)
			name:       "idempotent re-report no cooldown",
			reporterID: reporterID,
			reportedID: reportedID,
			matchID:    testMatchID,
			setupMock: func(repo *repomocks.MockCheatReportsRepository, cooldown *repomocks.MockCooldownStore) {
				repo.EXPECT().
					UpsertCheatReport(gomock.Any(), domain.CheatReport{ReporterID: reporterID, ReportedID: reportedID, MatchID: testMatchID}).
					Return(nil)
				repo.EXPECT().
					CountRecentCheatReports(gomock.Any(), reportedID, 10).
					Return(4, nil)
				// SetCooldown must NOT be called
			},
		},
		{
			// criterion: 2 — threshold not reached: count=4, no cooldown written
			name:       "threshold not reached no cooldown set",
			reporterID: reporterID,
			reportedID: reportedID,
			matchID:    testMatchID,
			setupMock: func(repo *repomocks.MockCheatReportsRepository, cooldown *repomocks.MockCooldownStore) {
				repo.EXPECT().
					UpsertCheatReport(gomock.Any(), gomock.Any()).
					Return(nil)
				repo.EXPECT().
					CountRecentCheatReports(gomock.Any(), reportedID, 10).
					Return(4, nil)
				// SetCooldown must NOT be called — if it is called the test fails
				cooldown.EXPECT().SetCooldown(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
		},
		{
			// criterion: 2 — threshold reached: count=5, cooldown written with 30-min TTL
			name:       "threshold reached cooldown set with 30min TTL",
			reporterID: reporterID,
			reportedID: reportedID,
			matchID:    testMatchID,
			setupMock: func(repo *repomocks.MockCheatReportsRepository, cooldown *repomocks.MockCooldownStore) {
				repo.EXPECT().
					UpsertCheatReport(gomock.Any(), gomock.Any()).
					Return(nil)
				repo.EXPECT().
					CountRecentCheatReports(gomock.Any(), reportedID, 10).
					Return(5, nil)
				cooldown.EXPECT().
					SetCooldown(gomock.Any(), reportedID, testCooldownTTL).
					Return(nil)
			},
		},
		{
			// criterion: 4 — self-report rejected with ErrSelfReport
			name:       "self-report returns ErrSelfReport",
			reporterID: reporterID,
			reportedID: reporterID, // same as reporterID
			matchID:    testMatchID,
			setupMock: func(repo *repomocks.MockCheatReportsRepository, cooldown *repomocks.MockCooldownStore) {
				// No repo calls expected for a self-report
				repo.EXPECT().UpsertCheatReport(gomock.Any(), gomock.Any()).Times(0)
				repo.EXPECT().CountRecentCheatReports(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
			wantErr: service.ErrSelfReport,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := repomocks.NewMockCheatReportsRepository(ctrl)
			cooldown := repomocks.NewMockCooldownStore(ctrl)

			tt.setupMock(repo, cooldown)

			svc := newService(t, repo, cooldown)
			err := svc.ReportCheat(context.Background(), tt.reporterID, tt.reportedID, tt.matchID)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ReportCheat() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("ReportCheat() unexpected error = %v", err)
			}
		})
	}
}

func TestGetCooldown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		userID     int64
		setupMock  func(cooldown *repomocks.MockCooldownStore)
		wantActive bool
		wantSecs   int
		wantErr    bool
	}{
		{
			// criterion: 3 — cooldown active: store returns true + seconds
			name:   "GetCooldown active",
			userID: reportedID,
			setupMock: func(cooldown *repomocks.MockCooldownStore) {
				cooldown.EXPECT().
					GetCooldown(gomock.Any(), reportedID).
					Return(true, 500, nil)
			},
			wantActive: true,
			wantSecs:   500,
		},
		{
			// criterion: 3 — cooldown inactive: store returns false + 0
			name:   "GetCooldown inactive",
			userID: reportedID,
			setupMock: func(cooldown *repomocks.MockCooldownStore) {
				cooldown.EXPECT().
					GetCooldown(gomock.Any(), reportedID).
					Return(false, 0, nil)
			},
			wantActive: false,
			wantSecs:   0,
		},
		{
			// criterion: 3 — store error is propagated
			name:   "GetCooldown store error propagated",
			userID: reportedID,
			setupMock: func(cooldown *repomocks.MockCooldownStore) {
				cooldown.EXPECT().
					GetCooldown(gomock.Any(), reportedID).
					Return(false, 0, errors.New("redis down"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := repomocks.NewMockCheatReportsRepository(ctrl)
			cooldown := repomocks.NewMockCooldownStore(ctrl)

			tt.setupMock(cooldown)

			svc := newService(t, repo, cooldown)
			status, err := svc.GetCooldown(context.Background(), tt.userID)

			if tt.wantErr {
				if err == nil {
					t.Fatal("GetCooldown() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("GetCooldown() unexpected error = %v", err)
			}

			if status.Active != tt.wantActive {
				t.Errorf("Active = %v, want %v", status.Active, tt.wantActive)
			}

			if status.SecondsRemaining != tt.wantSecs {
				t.Errorf("SecondsRemaining = %d, want %d", status.SecondsRemaining, tt.wantSecs)
			}
		})
	}
}

// TestReportCheat_ThresholdBoundary verifies the exact boundary: count=4 → no
// cooldown, count=5 → cooldown. This is the load-bearing test for criterion 2.
func TestReportCheat_ThresholdBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		count             int
		expectSetCooldown bool
	}{
		{
			// criterion: 2 — exactly at threshold-1, no cooldown
			name:              "count 4 below threshold no cooldown",
			count:             4,
			expectSetCooldown: false,
		},
		{
			// criterion: 2 — exactly at threshold, cooldown set
			name:              "count 5 at threshold cooldown set",
			count:             5,
			expectSetCooldown: true,
		},
		{
			// criterion: 2 — above threshold, cooldown still set
			name:              "count 6 above threshold cooldown set",
			count:             6,
			expectSetCooldown: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := repomocks.NewMockCheatReportsRepository(ctrl)
			cooldown := repomocks.NewMockCooldownStore(ctrl)

			repo.EXPECT().UpsertCheatReport(gomock.Any(), gomock.Any()).Return(nil)
			repo.EXPECT().CountRecentCheatReports(gomock.Any(), reportedID, 10).Return(tt.count, nil)

			if tt.expectSetCooldown {
				cooldown.EXPECT().SetCooldown(gomock.Any(), reportedID, testCooldownTTL).Return(nil)
			} else {
				cooldown.EXPECT().SetCooldown(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			}

			svc := newService(t, repo, cooldown)
			if err := svc.ReportCheat(context.Background(), reporterID, reportedID, testMatchID); err != nil {
				t.Fatalf("ReportCheat() error = %v", err)
			}
		})
	}
}
