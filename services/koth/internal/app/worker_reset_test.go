package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/koth/internal/api/domain"
	svcmocks "github.com/pizdagladki/full/services/koth/internal/api/service/mocks"
	"github.com/pizdagladki/full/services/koth/internal/config"
)

// TestCheckReset verifies criterion: the reset check invokes CloseStaleReign
// for BOTH the daily and monthly hills on every tick, off the request hot
// path, and a returned error is logged (non-fatal) rather than propagated —
// checkReset itself never returns an error.
func TestCheckReset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setupMock func(m *svcmocks.MockResetService)
	}{
		{
			// criterion: both HillTypeDaily and HillTypeMonthly are checked every tick
			name: "checks both daily and monthly hills",
			setupMock: func(m *svcmocks.MockResetService) {
				m.EXPECT().CloseStaleReign(gomock.Any(), domain.HillTypeDaily).Return(nil)
				m.EXPECT().CloseStaleReign(gomock.Any(), domain.HillTypeMonthly).Return(nil)
			},
		},
		{
			// criterion: an error from CloseStaleReign is swallowed (logged, non-fatal) —
			// checkReset must still call the other hill type and not panic/block.
			name: "a CloseStaleReign error is non-fatal and does not stop the other hill",
			setupMock: func(m *svcmocks.MockResetService) {
				m.EXPECT().CloseStaleReign(gomock.Any(), domain.HillTypeDaily).
					Return(errors.New("store unreachable"))
				m.EXPECT().CloseStaleReign(gomock.Any(), domain.HillTypeMonthly).Return(nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			resetMock := svcmocks.NewMockResetService(ctrl)
			tt.setupMock(resetMock)

			a := &App{logger: zap.NewNop(), resetSvc: resetMock}
			a.checkReset(context.Background())
		})
	}
}

// TestWorkerReset_ImmediateCheck verifies criterion: the reset job runs off
// the request hot path — it checks immediately on start (without waiting for
// the first tick) and returns cleanly when ctx is canceled.
func TestWorkerReset_ImmediateCheck(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	resetMock := svcmocks.NewMockResetService(ctrl)

	done := make(chan struct{})
	// Signal the first daily check via a buffered channel instead of a shared
	// counter — the mock callback runs on the workerReset goroutine, so a plain
	// int here is a data race under -race (same pattern as TestWorkerReset_TicksAgain).
	firstCheck := make(chan struct{}, 1)

	resetMock.EXPECT().CloseStaleReign(gomock.Any(), domain.HillTypeDaily).DoAndReturn(
		func(context.Context, domain.HillType) error {
			select {
			case firstCheck <- struct{}{}:
			default:
			}

			return nil
		}).AnyTimes()
	resetMock.EXPECT().CloseStaleReign(gomock.Any(), domain.HillTypeMonthly).DoAndReturn(
		func(context.Context, domain.HillType) error {
			return nil
		}).AnyTimes()

	a := &App{
		logger:   zap.NewNop(),
		resetSvc: resetMock,
		cfg:      &config.Config{Reset: config.ResetConfig{CheckInterval: time.Hour}},
	}

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		_ = workerReset(ctx, a)
		close(done)
	}()

	// The immediate check happens synchronously before the ticker loop — wait
	// for its signal (no sleep, no shared counter) without depending on the
	// (1-hour) ticker interval.
	select {
	case <-firstCheck:
	case <-time.After(2 * time.Second):
		t.Fatal("workerReset did not run an immediate check on start")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("workerReset did not return after ctx cancel")
	}
}

// TestWorkerReset_TicksAgain verifies criterion: the job runs periodically
// off a ticker at the configured interval, not just once — a short interval
// must produce more than the single immediate check within the test window.
func TestWorkerReset_TicksAgain(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	resetMock := svcmocks.NewMockResetService(ctrl)

	tickCh := make(chan struct{}, 100)

	resetMock.EXPECT().CloseStaleReign(gomock.Any(), domain.HillTypeDaily).DoAndReturn(
		func(context.Context, domain.HillType) error {
			select {
			case tickCh <- struct{}{}:
			default:
			}

			return nil
		}).AnyTimes()
	resetMock.EXPECT().CloseStaleReign(gomock.Any(), domain.HillTypeMonthly).Return(nil).AnyTimes()

	a := &App{
		logger:   zap.NewNop(),
		resetSvc: resetMock,
		cfg:      &config.Config{Reset: config.ResetConfig{CheckInterval: 10 * time.Millisecond}},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})

	go func() {
		_ = workerReset(ctx, a)
		close(done)
	}()

	seen := 0
	timeout := time.After(2 * time.Second)

	for seen < 2 {
		select {
		case <-tickCh:
			seen++
		case <-timeout:
			cancel()
			t.Fatal("workerReset did not tick at least twice within the timeout")
		}
	}

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("workerReset did not return after ctx cancel")
	}
}
