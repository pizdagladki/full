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

	"github.com/pizdagladki/full/services/matchmaking/internal/config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	a := New("matchmaking")
	if a == nil {
		t.Fatal("New returned nil")
	}
	if a.name != "matchmaking" {
		t.Errorf("name = %q, want %q", a.name, "matchmaking")
	}
}

func TestInitLogger(t *testing.T) {
	t.Parallel()

	a := New("matchmaking")
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

	a := New("matchmaking")
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

	// No IS_DOCKER -> file mode; cmd/config.yaml does not exist from this package's
	// working directory, so populateConfig (and thus Run) must fail.
	a := New("matchmaking")
	if err := a.Run(context.Background()); err == nil {
		t.Fatal("Run() error = nil, want a config-load error")
	}
}

func TestRun_FailsOnRedis(t *testing.T) {
	t.Setenv("IS_DOCKER", "1")
	t.Setenv("HTTP_ADDR", "127.0.0.1:0")
	t.Setenv("REDIS_ADDR", "127.0.0.1:1")

	a := New("matchmaking")
	if err := a.Run(context.Background()); err == nil {
		t.Fatal("Run() error = nil, want a Redis connect error")
	}
}

// --- worker_ws tests ---

func TestHealthz(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mux := buildMux(ctx)

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

func TestWS_PingAck(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mux := buildMux(ctx)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"

	c, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket.Dial: %v", err)
	}
	defer c.CloseNow() //nolint:errcheck // best-effort close in test

	if err := c.Write(ctx, websocket.MessageText, []byte("ping")); err != nil {
		t.Fatalf("Write(ping): %v", err)
	}

	_, msg, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(msg) != "pong" {
		t.Errorf("got %q, want %q", string(msg), "pong")
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

	// A bind address with no port makes ListenAndServe fail, surfacing a worker error.
	a := newTestApp("not-a-valid-bind-addr")

	if err := a.runWorkers(context.Background()); err == nil {
		t.Fatal("runWorkers() error = nil, want the worker's start error")
	}
}

// newTestApp builds an App wired with the minimum needed to exercise the WS
// worker: a no-op logger and an HTTP addr.
func newTestApp(addr string) *App {
	a := New("matchmaking-test")
	a.logger = zap.NewNop()
	a.cfg = &config.Config{HTTP: config.HTTPConfig{Addr: addr}}

	return a
}
