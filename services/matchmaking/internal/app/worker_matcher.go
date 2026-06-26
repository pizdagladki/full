package app

import (
	"context"
	"time"
)

const matcherTickInterval = 500 * time.Millisecond

// workerMatcher runs the pairing loop on a fixed interval until ctx is canceled.
func workerMatcher(ctx context.Context, a *App) error {
	ticker := time.NewTicker(matcherTickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			a.matchmakingSvc.Tick(ctx)
		}
	}
}
