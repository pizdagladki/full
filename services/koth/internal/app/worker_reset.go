package app

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/koth/internal/api/domain"
)

// workerReset runs the daily/monthly reset job off the request hot path: on
// a ticker at a.cfg.Reset.CheckInterval (and once immediately at start), it
// checks both hills for a rolled-over day/month boundary and closes any
// stale reign — awarding the final-placement reward and expiring the king
// clip. Errors are logged (non-fatal) and the loop continues; it returns nil
// cleanly when ctx is canceled.
func workerReset(ctx context.Context, a *App) error {
	a.checkReset(ctx)

	ticker := time.NewTicker(a.cfg.Reset.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			a.checkReset(ctx)
		}
	}
}

// checkReset runs one reset-check tick for both the daily and monthly hills.
func (a *App) checkReset(ctx context.Context) {
	for _, hillType := range []domain.HillType{domain.HillTypeDaily, domain.HillTypeMonthly} {
		err := a.resetSvc.CloseStaleReign(ctx, hillType)
		if err != nil {
			a.logger.Warn("reset worker: close stale reign failed",
				zap.String("hill_type", string(hillType)), zap.Error(err))
		}
	}
}
