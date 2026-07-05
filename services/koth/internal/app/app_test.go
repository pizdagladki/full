package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/koth/internal/api/delivery"
	"github.com/pizdagladki/full/services/koth/internal/api/domain"
	"github.com/pizdagladki/full/services/koth/internal/api/middleware"
	svcmocks "github.com/pizdagladki/full/services/koth/internal/api/service/mocks"
	"github.com/pizdagladki/full/services/koth/internal/config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	a := New("koth")
	if a == nil {
		t.Fatal("New returned nil")
	}
	if a.name != "koth" {
		t.Errorf("name = %q, want %q", a.name, "koth")
	}
}

func TestInitLogger(t *testing.T) {
	t.Parallel()

	a := New("koth")
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

	a := New("koth")
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

// TestRegisterHTTPRoutes_Healthz verifies criterion: GET /healthz returns 200
// with a JSON {"status":"ok"} body, and that the router carries the
// validator/v10 wrapper.
func TestRegisterHTTPRoutes_Healthz(t *testing.T) {
	t.Parallel()

	a := newTestApp(t, "127.0.0.1:0")
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
	if body := rec.Body.String(); body != `{"status":"ok"}`+"\n" {
		t.Errorf("body = %q, want %q", body, `{"status":"ok"}`+"\n")
	}
}

// TestInitPostgres_Error verifies criterion: a malformed Postgres DSN aborts
// startup with an error, and pgxPool stays nil (never a live pool alongside an
// error).
func TestInitPostgres_Error(t *testing.T) {
	t.Parallel()

	a := New("koth")
	a.logger = zap.NewNop()
	a.cfg = &config.Config{Postgres: config.PostgresConfig{DSN: "postgres://localhost:not-a-port/db"}}

	if err := a.initPostgres(context.Background()); err == nil {
		t.Fatal("initPostgres() error = nil, want error for a malformed DSN")
	}
	if a.pgxPool != nil {
		t.Error("pgxPool is non-nil after a failed connect")
	}
}

// TestInitRedis_Error verifies criterion: an unreachable Redis address aborts
// startup with an error, and redisClient stays nil.
func TestInitRedis_Error(t *testing.T) {
	t.Parallel()

	a := New("koth")
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

// TestRunWorkers_GracefulShutdown verifies criterion: runWorkers returns nil
// (no error) after the context is canceled — a clean graceful shutdown.
func TestRunWorkers_GracefulShutdown(t *testing.T) {
	t.Parallel()

	a := newTestApp(t, "127.0.0.1:0")

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

// TestRunWorkers_WorkerError verifies criterion: a worker start failure (bad
// bind address) is surfaced as an error from runWorkers. It also proves that
// runWorkers itself unwinds workerReset's ctx.Done()-only loop and returns
// promptly, without relying on the caller to cancel the passed-in ctx.
func TestRunWorkers_WorkerError(t *testing.T) {
	t.Parallel()

	// A bind address with no port makes e.Start fail, surfacing a worker error.
	a := newTestApp(t, "not-a-valid-bind-addr")

	err := a.runWorkers(context.Background())
	if err == nil {
		t.Fatal("runWorkers() error = nil, want the worker's start error")
	}
}

// TestRunWorkers_CancelsSharedCtxOnFirstExit verifies criterion: when one
// worker returns (with an error) while another worker's loop only exits on
// ctx.Done(), runWorkers cancels the shared ctx as soon as the first worker
// exits, unwinding the still-running worker so runWorkers returns promptly
// instead of hanging on wg.Wait() forever.
func TestRunWorkers_CancelsSharedCtxOnFirstExit(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("boom")

	failFast := func(_ context.Context, _ *App) error {
		return wantErr
	}

	blockUntilCanceled := func(ctx context.Context, _ *App) error {
		<-ctx.Done()

		return nil
	}

	a := &App{}

	done := make(chan error, 1)
	go func() {
		done <- a.runWorkersFor(context.Background(), []worker{failFast, blockUntilCanceled})
	}()

	select {
	case err := <-done:
		if !errors.Is(err, wantErr) {
			t.Fatalf("runWorkers() error = %v, want %v", err, wantErr)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("runWorkers did not return promptly after one worker exited — shared ctx was not canceled")
	}
}

// TestRun_FailsOnConfig verifies criterion: Run aborts with an error when the
// config cannot be loaded (no IS_DOCKER and no cmd/config.yaml present).
func TestRun_FailsOnConfig(t *testing.T) {
	t.Parallel()

	// No IS_DOCKER -> file mode; cmd/config.yaml does not exist from this package's
	// working directory, so populateConfig (and thus Run) must fail.
	a := New("koth")
	if err := a.Run(context.Background()); err == nil {
		t.Fatal("Run() error = nil, want a config-load error")
	}
}

// TestRun_FailsOnPostgres verifies criterion: Run aborts with an error when
// Postgres is unreachable/malformed, even though config and Redis addr are
// otherwise well-formed.
func TestRun_FailsOnPostgres(t *testing.T) {
	t.Setenv("IS_DOCKER", "1")
	t.Setenv("HTTP_ADDR", "127.0.0.1:0")
	t.Setenv("POSTGRES_DSN", "postgres://localhost:not-a-port/db")
	t.Setenv("REDIS_ADDR", "127.0.0.1:6379")

	a := New("koth")
	if err := a.Run(context.Background()); err == nil {
		t.Fatal("Run() error = nil, want a Postgres connect error")
	}
}

// newTestApp builds an App wired with the minimum needed to exercise the HTTP
// worker and router: a no-op logger, the validator, an HTTP addr, and stub
// handlers/middleware so registerHTTPRoutes works without a live database.
func newTestApp(t *testing.T, addr string) *App {
	t.Helper()

	ctrl := gomock.NewController(t)
	rankMock := svcmocks.NewMockRankService(ctrl)
	sessionMock := svcmocks.NewMockSessionService(ctrl)

	a := New("koth-test")
	a.logger = zap.NewNop()
	a.initValidator()
	a.cfg = &config.Config{
		HTTP:    config.HTTPConfig{Addr: addr},
		Session: config.SessionConfig{CookieName: "session"},
		// Long interval: workerReset's first (immediate) tick fires once via
		// checkReset in workerReset's own call, and the ticker itself won't
		// fire again within these tests' short lifetimes.
		Reset: config.ResetConfig{CheckInterval: time.Hour},
	}

	hillMock := svcmocks.NewMockHillService(ctrl)
	resetMock := svcmocks.NewMockResetService(ctrl)
	resetMock.EXPECT().CloseStaleReign(gomock.Any(), domain.HillTypeDaily).Return(nil).AnyTimes()
	resetMock.EXPECT().CloseStaleReign(gomock.Any(), domain.HillTypeMonthly).Return(nil).AnyTimes()

	// Wire stub handlers and middleware so registerHTTPRoutes works.
	a.rankHandler = delivery.NewRankHandler(rankMock, zap.NewNop())
	a.hillHandler = delivery.NewHillHandler(hillMock, zap.NewNop())
	a.authMiddleware = middleware.NewAuthMiddleware(sessionMock, "session", zap.NewNop())
	a.resetSvc = resetMock

	return a
}
