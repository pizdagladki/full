package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/media/internal/api/delivery"
	"github.com/pizdagladki/full/services/media/internal/api/middleware"
	"github.com/pizdagladki/full/services/media/internal/config"
)

// noopSessionSvc is a minimal SessionService for tests that never resolves a
// session — it is used only to satisfy the AuthMiddleware constructor so that
// registerHTTPRoutes does not panic on a nil middleware.
type noopSessionSvc struct{}

func (noopSessionSvc) ResolveSession(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

// noopClipHandler is a minimal ClipHandler for tests that only exercise the
// HTTP worker lifecycle, not the clip routes.
type noopClipHandler struct{}

func (noopClipHandler) Upload(c echo.Context) error   { return c.NoContent(http.StatusNotImplemented) }
func (noopClipHandler) List(c echo.Context) error     { return c.NoContent(http.StatusNotImplemented) }
func (noopClipHandler) Download(c echo.Context) error { return c.NoContent(http.StatusNotImplemented) }

var _ delivery.ClipHandler = noopClipHandler{}

func TestNew(t *testing.T) {
	t.Parallel()

	a := New("media")
	if a == nil {
		t.Fatal("New returned nil")
	}
	if a.name != "media" {
		t.Errorf("name = %q, want %q", a.name, "media")
	}
}

func TestInitLogger(t *testing.T) {
	t.Parallel()

	a := New("media")
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

	a := New("media")
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

func TestPopulateConfig_MissingFile(t *testing.T) {
	t.Parallel()

	a := New("media")
	a.logger = zap.NewNop()
	// No IS_DOCKER -> file mode; cmd/config.yaml does not exist from this
	// package's working directory, so populateConfig must fail.
	if err := a.populateConfig(); err == nil {
		t.Fatal("populateConfig() error = nil, want a config-load error")
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

	a := New("media")
	a.logger = zap.NewNop()
	a.cfg = &config.Config{Postgres: config.PostgresConfig{DSN: "postgres://localhost:not-a-port/db"}}

	if err := a.initPostgres(context.Background()); err == nil {
		t.Fatal("initPostgres() error = nil, want error for a malformed DSN")
	}
	if a.pgxPool != nil {
		t.Error("pgxPool is non-nil after a failed connect")
	}
}

func TestInitStorage_Error(t *testing.T) {
	t.Parallel()

	a := New("media")
	a.logger = zap.NewNop()
	a.cfg = &config.Config{
		Storage: config.StorageConfig{
			Endpoint:  "127.0.0.1:1",
			AccessKey: "minioadmin",
			SecretKey: "minioadmin",
			Bucket:    "test-bucket",
			UseSSL:    false,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := a.initStorage(ctx); err == nil {
		t.Fatal("initStorage() error = nil, want error against an unreachable MinIO endpoint")
	}
	if a.minioClient != nil {
		t.Error("minioClient is non-nil after a failed connect")
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
	a := New("media")
	if err := a.Run(context.Background()); err == nil {
		t.Fatal("Run() error = nil, want a config-load error")
	}
}

func TestRun_FailsOnPostgres(t *testing.T) {
	t.Setenv("IS_DOCKER", "1")
	t.Setenv("HTTP_ADDR", "127.0.0.1:0")
	t.Setenv("POSTGRES_DSN", "postgres://localhost:not-a-port/db")
	t.Setenv("STORAGE_ENDPOINT", "127.0.0.1:9000")
	t.Setenv("STORAGE_ACCESS_KEY", "minioadmin")
	t.Setenv("STORAGE_SECRET_KEY", "minioadmin")
	t.Setenv("STORAGE_BUCKET", "media")
	t.Setenv("REDIS_ADDR", "localhost:6379")

	a := New("media")
	if err := a.Run(context.Background()); err == nil {
		t.Fatal("Run() error = nil, want a Postgres connect error")
	}
}

// newTestApp builds an App wired with the minimum needed to exercise the HTTP
// worker and router: a no-op logger, the validator, an HTTP addr, and a no-op
// auth middleware so registerHTTPRoutes does not panic on a nil middleware.
func newTestApp(addr string) *App {
	a := New("media-test")
	a.logger = zap.NewNop()
	a.initValidator()
	a.cfg = &config.Config{
		HTTP:    config.HTTPConfig{Addr: addr},
		Session: config.SessionConfig{CookieName: "session"},
	}
	a.authMiddleware = middleware.NewAuthMiddleware(noopSessionSvc{}, "session", zap.NewNop())
	a.clipHandler = noopClipHandler{}

	return a
}
