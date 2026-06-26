package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// workerWS runs the net/http server exposing /healthz and /ws, and shuts it
// down gracefully on ctx.Done().
func workerWS(ctx context.Context, a *App) error {
	mux := buildMux(a)

	srv := &http.Server{
		Addr:              a.cfg.HTTP.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(_ net.Listener) context.Context { return ctx },
	}

	errCh := make(chan error, 1)

	go func() {
		a.logger.Info("ws server listening", zap.String("addr", a.cfg.HTTP.Addr))

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

// buildMux constructs the HTTP multiplexer with the /healthz and /ws routes.
func buildMux(a *App) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", handleHealthz)
	mux.HandleFunc("/ws", a.wsHandler.ServeWS)

	return mux
}

// handleHealthz is the liveness probe: it reports that the process is up.
func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
