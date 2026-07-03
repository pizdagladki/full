package app

import (
	"context"
	"sync"
)

// worker is a background task bound to the App.
type worker func(ctx context.Context, a *App) error

// runWorkers starts every worker as a goroutine under a shared WaitGroup and
// blocks until they all finish, returning the first error seen. As soon as
// ANY worker's goroutine returns (with or without an error), the shared ctx
// is canceled so the remaining workers — which only exit on ctx.Done() —
// unwind promptly instead of leaving wg.Wait() blocked forever.
func (a *App) runWorkers(ctx context.Context) error {
	return a.runWorkersFor(ctx, []worker{workerHTTP, workerReset})
}

// runWorkersFor is runWorkers parameterized over the worker list, so tests
// can exercise the cancel-on-first-exit behavior with fake workers instead
// of the real HTTP/reset workers.
func (a *App) runWorkersFor(ctx context.Context, workers []worker) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
	)

	for _, w := range workers {
		wg.Add(1)

		go func(w worker) {
			defer wg.Done()
			defer cancel()

			err := w(ctx, a)
			if err != nil {
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
