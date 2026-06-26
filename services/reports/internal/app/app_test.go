package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/reports/internal/config"
)

// TestNew verifies that New returns a non-nil App with the given name.
// criterion: 1 — services/reports/ exists with canonical layout
func TestNew(t *testing.T) {
	t.Parallel()

	a := New("reports")
	if a == nil {
		t.Fatal("New returned nil")
	}
	if a.name != "reports" {
		t.Errorf("name = %q, want %q", a.name, "reports")
	}
}

// TestInitLogger verifies that initLogger sets a non-nil logger.
// criterion: 1 — canonical app assembly layer
func TestInitLogger(t *testing.T) {
	t.Parallel()

	a := New("reports")
	if err := a.initLogger(); err != nil {
		t.Fatalf("initLogger() error = %v", err)
	}
	if a.logger == nil {
		t.Fatal("logger is nil after initLogger")
	}
	_ = a.logger.Sync()
}

// TestInitValidator verifies that initValidator wires a working validator/v10 wrapper.
// criterion: 5 — e.Validator is wired to the validator/v10 wrapper
func TestInitValidator(t *testing.T) {
	t.Parallel()

	a := New("reports")
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

// TestRegisterHTTPRoutes_Healthz verifies that GET /healthz returns 200 and
// that e.Validator is set.
// criterion: 2 — GET /healthz → 200
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

// TestInitPostgres_Error verifies that a bad DSN aborts startup without leaving
// a non-nil pgxPool.
// criterion: 4 — failed Postgres ping aborts startup
func TestInitPostgres_Error(t *testing.T) {
	t.Parallel()

	a := New("reports")
	a.logger = zap.NewNop()
	a.cfg = &config.Config{Postgres: config.PostgresConfig{DSN: "postgres://localhost:not-a-port/db"}}

	if err := a.initPostgres(context.Background()); err == nil {
		t.Fatal("initPostgres() error = nil, want error for a malformed DSN")
	}
	if a.pgxPool != nil {
		t.Error("pgxPool is non-nil after a failed connect")
	}
}

// TestInitRedis_Error verifies that an unreachable Redis aborts startup without
// leaving a non-nil redisClient.
// criterion: 4 — failed Redis ping aborts startup
func TestInitRedis_Error(t *testing.T) {
	t.Parallel()

	a := New("reports")
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

// TestRunWorkers_GracefulShutdown verifies that runWorkers returns nil when ctx
// is cancelled (graceful shutdown path).
// criterion: 5 — e.Shutdown(ctx) runs on context cancel
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

// TestRunWorkers_WorkerError verifies that runWorkers propagates a worker's
// start error.
// criterion: 2 — app.Run(ctx) is functional
func TestRunWorkers_WorkerError(t *testing.T) {
	t.Parallel()

	// A bind address with no port makes e.Start fail, surfacing a worker error.
	a := newTestApp("not-a-valid-bind-addr")

	if err := a.runWorkers(context.Background()); err == nil {
		t.Fatal("runWorkers() error = nil, want the worker's start error")
	}
}

// TestRun_FailsOnConfig verifies that Run fails when config cannot be loaded
// (no config.yaml in the working directory).
// criterion: 3 — ValidateConfig fails startup when required fields are unset
func TestRun_FailsOnConfig(t *testing.T) {
	t.Parallel()

	// No IS_DOCKER -> file mode; cmd/config.yaml does not exist from this
	// package's working directory, so populateConfig (and thus Run) must fail.
	a := New("reports")
	if err := a.Run(context.Background()); err == nil {
		t.Fatal("Run() error = nil, want a config-load error")
	}
}

// TestRun_FailsOnPostgres verifies that Run aborts when Postgres is unreachable.
// criterion: 4 — failed Postgres ping aborts startup with a logged error
func TestRun_FailsOnPostgres(t *testing.T) {
	t.Setenv("IS_DOCKER", "1")
	t.Setenv("HTTP_ADDR", "127.0.0.1:0")
	t.Setenv("POSTGRES_DSN", "postgres://localhost:not-a-port/db")
	t.Setenv("REDIS_ADDR", "127.0.0.1:6379")

	a := New("reports")
	if err := a.Run(context.Background()); err == nil {
		t.Fatal("Run() error = nil, want a Postgres connect error")
	}
}

// newTestApp builds an App wired with the minimum needed to exercise the HTTP
// worker and router: a no-op logger, the validator, and an HTTP addr.
func newTestApp(addr string) *App {
	a := New("reports-test")
	a.logger = zap.NewNop()
	a.initValidator()
	a.cfg = &config.Config{HTTP: config.HTTPConfig{Addr: addr}}

	return a
}
