package app

import (
	"context"
	"errors"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// workerHTTP runs the Echo HTTP server and shuts it down gracefully on ctx.Done().
func workerHTTP(ctx context.Context, a *App) error {
	e := a.registerHTTPRoutes()
	e.Server.ReadHeaderTimeout = 10 * time.Second

	errCh := make(chan error, 1)

	go func() {
		a.logger.Info("http server listening", zap.String("addr", a.cfg.HTTP.Addr))

		err := e.Start(a.cfg.HTTP.Addr)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err

			return
		}

		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		return e.Shutdown(shutdownCtx) //nolint:contextcheck // intentional: fresh shutdown ctx
	case err := <-errCh:
		return err
	}
}
