package service_test

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/pizdagladki/full/services/koth/internal/api/domain"
	repomocks "github.com/pizdagladki/full/services/koth/internal/api/repository/mocks"
	"github.com/pizdagladki/full/services/koth/internal/api/service"

	"github.com/pizdagladki/full/services/koth/internal/api/repository"
)

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

			svc := service.NewHillService(repoMock)

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

// TestHillService_Challenge verifies criteria 2, 3, 5, and 6 — Challenge
// parses hill_type, delegates the decide-and-transfer to the repository, and
// propagates its outcome (won/lost) and errors unchanged.
func TestHillService_Challenge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		hillType   string
		userID     int64
		survivedMs int
		newClipID  string
		setupRepo  func(m *repomocks.MockHillRepository)
		wantWon    bool
		wantErr    error
	}{
		{
			// criterion: 2 — a winning challenge (survived_ms >= king.blink_ts_ms) reports won=true
			// with the challenger's own clip + survived_ms as the new blink_ts_ms
			name:       "challenger wins takes crown",
			hillType:   "daily",
			userID:     99,
			survivedMs: 9000,
			newClipID:  "clip-new",
			setupRepo: func(m *repomocks.MockHillRepository) {
				m.EXPECT().Challenge(gomock.Any(), domain.HillTypeDaily, int64(99), 9000, "clip-new").
					Return(domain.ChallengeOutcome{
						Won:  true,
						King: domain.KingReign{UserID: 99, ClipID: "clip-new", BlinkTsMs: 9000},
					}, nil)
			},
			wantWon: true,
		},
		{
			// criterion: 2 — a losing challenge (survived_ms < king.blink_ts_ms) reports
			// won=false and the current (unchanged) king
			name:       "challenger loses king stays",
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
			wantWon: false,
		},
		{
			// criterion: 5 — an invalid hill_type is rejected before hitting the repository
			name:      "invalid hill_type returns ErrInvalidHillType",
			hillType:  "weekly",
			setupRepo: func(_ *repomocks.MockHillRepository) {},
			wantErr:   domain.ErrInvalidHillType,
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
			wantErr: repository.ErrHillNotFound,
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
			wantErr: errors.New("db error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repoMock := repomocks.NewMockHillRepository(ctrl)
			tt.setupRepo(repoMock)

			svc := service.NewHillService(repoMock)

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
