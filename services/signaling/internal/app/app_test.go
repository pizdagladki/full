package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/signaling/internal/api/delivery"
	"github.com/pizdagladki/full/services/signaling/internal/config"
)

// newTestApp builds an App with the minimum fields needed to test the WS worker.
// It uses a stub SignalingHandler so it does not need Redis/repos/services.
func newTestApp(addr string) *App {
	a := New("signaling-test")
	a.logger = zap.NewNop()
	a.cfg = &config.Config{
		HTTP:      config.HTTPConfig{Addr: addr},
		Signaling: config.SignalingConfig{SessionCookie: "session"},
	}
	// Wire a minimal stub handler that closes the connection immediately.
	a.wsHandler = &stubSignalingHandler{}

	return a
}

// stubSignalingHandler immediately closes any incoming WS connection.
// Used in app-level tests that only need the HTTP server to start.
type stubSignalingHandler struct{}

func (h *stubSignalingHandler) ServeWS(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}

	defer c.CloseNow() //nolint:errcheck

	// Just block until the context is done.
	<-r.Context().Done()
}

// Ensure stubSignalingHandler satisfies the interface.
var _ delivery.SignalingHandler = (*stubSignalingHandler)(nil)

func TestNew(t *testing.T) {
	t.Parallel()

	a := New("signaling")
	if a == nil {
		t.Fatal("New returned nil")
	}

	if a.name != "signaling" {
		t.Errorf("name = %q, want %q", a.name, "signaling")
	}
}

func TestInitLogger(t *testing.T) {
	t.Parallel()

	a := New("signaling")
	if err := a.initLogger(); err != nil {
		t.Fatalf("initLogger() error = %v", err)
	}

	if a.logger == nil {
		t.Fatal("logger is nil after initLogger")
	}

	_ = a.logger.Sync()
}

func TestInitRedis_Error(t *testing.T) {
	t.Parallel()

	a := New("signaling")
	a.logger = zap.NewNop()
	a.cfg = &config.Config{Redis: config.RedisConfig{Addr: "127.0.0.1:1"}}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := a.initRedis(ctx); err == nil {
		t.Fatal("initRedis() error = nil, want error against an unreachable Redis")
	}

	if a.redisClient != nil {
		t.Error("redisClient is non-nil after a failed connect")
	}
}

func TestRun_FailsOnConfig(t *testing.T) {
	t.Parallel()

	// No IS_DOCKER → file mode; cmd/config.yaml does not exist from this
	// package's working directory, so populateConfig (and thus Run) must fail.
	a := New("signaling")
	if err := a.Run(context.Background()); err == nil {
		t.Fatal("Run() error = nil, want a config-load error")
	}
}

func TestRun_FailsOnRedis(t *testing.T) {
	t.Setenv("IS_DOCKER", "1")
	t.Setenv("HTTP_ADDR", "127.0.0.1:0")
	t.Setenv("REDIS_ADDR", "127.0.0.1:1")

	a := New("signaling")
	if err := a.Run(context.Background()); err == nil {
		t.Fatal("Run() error = nil, want a Redis connect error")
	}
}

// --- worker_ws tests ---

func TestHealthz(t *testing.T) {
	t.Parallel()

	a := newTestApp("127.0.0.1:0")
	mux := buildMux(a)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	if body := rec.Body.String(); body != `{"status":"ok"}` {
		t.Errorf("body = %q, want %q", body, `{"status":"ok"}`)
	}
}

func TestWorkerWS_GracefulShutdown(t *testing.T) {
	t.Parallel()

	a := newTestApp("127.0.0.1:0")

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- a.runWorkers(ctx) }()

	// Give the server a moment to start, then trigger graceful shutdown.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runWorkers() error = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runWorkers did not return after ctx cancel")
	}
}

func TestWorkerWS_StartError(t *testing.T) {
	t.Parallel()

	// A bind address with no port makes ListenAndServe fail.
	a := newTestApp("not-a-valid-bind-addr")

	if err := a.runWorkers(context.Background()); err == nil {
		t.Fatal("runWorkers() error = nil, want the worker's start error")
	}
}

// TestWS_CtxCancelClosesConn asserts that cancelling the server context causes
// the handler to return and the client's next Read to error.
func TestWS_CtxCancelClosesConn(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a := newTestApp("127.0.0.1:0")

	// Use httptest.NewServer to pick a free port; BaseContext is not exposed via
	// httptest directly, so we test via the stub handler's context-wait behaviour.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Inject the cancellable context into the request.
		r = r.WithContext(ctx)
		a.wsHandler.ServeWS(w, r)
	}))
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"

	dialCtx := context.Background()

	c, _, err := websocket.Dial(dialCtx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket.Dial: %v", err)
	}

	defer c.CloseNow() //nolint:errcheck

	// Cancel the server-side context.
	cancel()

	readDone := make(chan error, 1)

	go func() {
		_, _, readErr := c.Read(dialCtx)
		readDone <- readErr
	}()

	select {
	case readErr := <-readDone:
		if readErr == nil {
			t.Fatal("Read returned nil after ctx cancel, want an error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("client Read did not return an error within 5s after server ctx cancel")
	}
}
