package app

import (
	"context"
	"sync"
)

// worker is a background task bound to the App.
type worker func(ctx context.Context, a *App) error

// runWorkers starts every worker as a goroutine under a shared WaitGroup and
// blocks until they all finish, returning the first error seen.
func (a *App) runWorkers(ctx context.Context) error {
	workers := []worker{workerHTTP}

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
	)
	for _, w := range workers {
		wg.Add(1)
		go func(w worker) {
			defer wg.Done()
			if err := w(ctx, a); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}(w)
	}
	wg.Wait()
	return firstErr
}
