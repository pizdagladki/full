package app

import (
	"context"
	"errors"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// workerHTTP runs the HTTP server and shuts it down gracefully on ctx.Done().
func workerHTTP(ctx context.Context, a *App) error {
	srv := &http.Server{
		Addr:              a.cfg.HTTP.Addr,
		Handler:           a.registerHTTPRoutes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)

	go func() {
		a.logger.Info("http server listening", zap.String("addr", srv.Addr))

		err := srv.ListenAndServe()
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

		return srv.Shutdown(shutdownCtx) //nolint:contextcheck // intentional: fresh shutdown ctx
	case err := <-errCh:
		return err
	}
}
