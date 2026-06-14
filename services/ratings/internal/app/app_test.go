package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/ratings/internal/config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	a := New("ratings")
	if a == nil {
		t.Fatal("New returned nil")
	}
	if a.name != "ratings" {
		t.Errorf("name = %q, want %q", a.name, "ratings")
	}
}

func TestInitLogger(t *testing.T) {
	t.Parallel()

	a := New("ratings")
	if err := a.initLogger(); err != nil {
		t.Fatalf("initLogger() error = %v", err)
	}
	if a.logger == nil {
		t.Fatal("logger is nil after initLogger")
	}
	_ = a.logger.Sync()
}

func TestInitValidator(t *testing.T) {
	t.Parallel()

	a := New("ratings")
	a.initValidator()
	if a.validator == nil {
		t.Fatal("validator is nil after initValidator")
	}

	type payload struct {
		Name string `validate:"required"`
	}

	if err := a.validator.Validate(payload{Name: "set"}); err != nil {
		t.Errorf("Validate(valid) error = %v, want nil", err)
	}
	if err := a.validator.Validate(payload{}); err == nil {
		t.Error("Validate(invalid) error = nil, want error")
	}
}

func TestRegisterHTTPRoutes_Healthz(t *testing.T) {
	t.Parallel()

	a := newTestApp("127.0.0.1:0")
	e := a.registerHTTPRoutes()

	if e.Validator == nil {
		t.Error("e.Validator is nil, want the validator/v10 wrapper")
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get(echo.HeaderContentType); ct != echo.MIMEApplicationJSON {
		t.Errorf("Content-Type = %q, want %q", ct, echo.MIMEApplicationJSON)
	}
}

func TestInitPostgres_Error(t *testing.T) {
	t.Parallel()

	a := New("ratings")
	a.logger = zap.NewNop()
	a.cfg = &config.Config{Postgres: config.PostgresConfig{DSN: "postgres://localhost:not-a-port/db"}}

	if err := a.initPostgres(context.Background()); err == nil {
		t.Fatal("initPostgres() error = nil, want error for a malformed DSN")
	}
	if a.pgxPool != nil {
		t.Error("pgxPool is non-nil after a failed connect")
	}
}

func TestInitRedis_Error(t *testing.T) {
	t.Parallel()

	a := New("ratings")
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

func TestRunWorkers_GracefulShutdown(t *testing.T) {
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

func TestRunWorkers_WorkerError(t *testing.T) {
	t.Parallel()

	// A bind address with no port makes e.Start fail, surfacing a worker error.
	a := newTestApp("not-a-valid-bind-addr")

	if err := a.runWorkers(context.Background()); err == nil {
		t.Fatal("runWorkers() error = nil, want the worker's start error")
	}
}

func TestRun_FailsOnConfig(t *testing.T) {
	t.Parallel()

	// No IS_DOCKER -> file mode; cmd/config.yaml does not exist from this package's
	// working directory, so populateConfig (and thus Run) must fail.
	a := New("ratings")
	if err := a.Run(context.Background()); err == nil {
		t.Fatal("Run() error = nil, want a config-load error")
	}
}

func TestRun_FailsOnPostgres(t *testing.T) {
	t.Setenv("IS_DOCKER", "1")
	t.Setenv("HTTP_ADDR", "127.0.0.1:0")
	t.Setenv("POSTGRES_DSN", "postgres://localhost:not-a-port/db")
	t.Setenv("REDIS_ADDR", "127.0.0.1:6379")

	a := New("ratings")
	if err := a.Run(context.Background()); err == nil {
		t.Fatal("Run() error = nil, want a Postgres connect error")
	}
}

// newTestApp builds an App wired with the minimum needed to exercise the HTTP
// worker and router: a no-op logger, the validator, and an HTTP addr.
func newTestApp(addr string) *App {
	a := New("ratings-test")
	a.logger = zap.NewNop()
	a.initValidator()
	a.cfg = &config.Config{HTTP: config.HTTPConfig{Addr: addr}}

	return a
}
