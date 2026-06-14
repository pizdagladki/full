package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"go.uber.org/zap"
)

// workerWS runs the net/http server exposing /healthz and /ws, and shuts it
// down gracefully on ctx.Done().
func workerWS(ctx context.Context, a *App) error {
	mux := buildMux(ctx)

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
func buildMux(ctx context.Context) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", handleHealthz)
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleWS(ctx, w, r)
	})

	return mux
}

// handleHealthz is the liveness probe: it reports that the process is up.
func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// handleWS accepts a WebSocket connection and implements a simple ping-pong
// application-level acknowledgement: when the client sends "ping", the server
// replies "pong". The loop exits cleanly when the client closes or the context
// is canceled.
func handleWS(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	// nil options uses coder/websocket's secure default: same-origin check is enforced and
	// InsecureSkipVerify is deliberately NOT set. Cross-origin support must be added later via
	// an explicit AcceptOptions.OriginPatterns allow-list — never by setting InsecureSkipVerify.
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer c.CloseNow() //nolint:errcheck // best-effort close

	for {
		_, msg, err := c.Read(ctx)
		if err != nil {
			return
		}

		if string(msg) == "ping" {
			writeErr := c.Write(ctx, websocket.MessageText, []byte("pong"))
			if writeErr != nil {
				return
			}
		}
	}
}
