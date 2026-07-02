package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/store/internal/api/delivery"
	"github.com/pizdagladki/full/services/store/internal/api/middleware"
	svcmocks "github.com/pizdagladki/full/services/store/internal/api/service/mocks"
	"github.com/pizdagladki/full/services/store/internal/config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	a := New("store")
	if a == nil {
		t.Fatal("New returned nil")
	}
	if a.name != "store" {
		t.Errorf("name = %q, want %q", a.name, "store")
	}
}

func TestInitLogger(t *testing.T) {
	t.Parallel()

	a := New("store")
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

	a := New("store")
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
}

func TestInitPostgres_Error(t *testing.T) {
	t.Parallel()

	a := New("store")
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

	a := New("store")
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

// TestInitRepositories verifies that initRepositories wires all repos using nil
// pools — construction must not deref, so nil is safe here.
func TestInitRepositories(t *testing.T) {
	t.Parallel()

	a := New("store")
	// pgxPool and redisClient left nil; constructors only store the dep,
	// they never dereference it during construction.
	a.initRepositories()

	if a.catalogRepo == nil {
		t.Fatal("catalogRepo is nil after initRepositories")
	}
	if a.inventoryRepo == nil {
		t.Fatal("inventoryRepo is nil after initRepositories")
	}
	if a.sessionRepo == nil {
		t.Fatal("sessionRepo is nil after initRepositories")
	}
	if a.purchaseRepo == nil {
		t.Fatal("purchaseRepo is nil after initRepositories")
	}
	if a.pointsRepo == nil {
		t.Fatal("pointsRepo is nil after initRepositories")
	}
	if a.pointsCache == nil {
		t.Fatal("pointsCache is nil after initRepositories")
	}
}

// TestInitServices verifies that initServices wires all services from repos.
func TestInitServices(t *testing.T) {
	t.Parallel()

	a := New("store")
	a.logger = zap.NewNop()
	a.cfg = &config.Config{
		Stripe: config.StripeConfig{
			SecretKey:            "sk_test_key",
			WebhookSigningSecret: "whsec_secret",
		},
	}
	a.initRepositories() // wires repo wrappers around nil pools (nil-safe construction)
	a.initServices()

	if a.catalogSvc == nil {
		t.Error("catalogSvc is nil after initServices")
	}
	if a.inventorySvc == nil {
		t.Error("inventorySvc is nil after initServices")
	}
	if a.sessionSvc == nil {
		t.Error("sessionSvc is nil after initServices")
	}
	if a.purchaseSvc == nil {
		t.Error("purchaseSvc is nil after initServices")
	}
	if a.paymentProvider == nil {
		t.Error("paymentProvider is nil after initServices")
	}
	if a.pointsSvc == nil {
		t.Error("pointsSvc is nil after initServices")
	}
}

// TestInitHandlers verifies that initHandlers wires storeHandler and purchaseHandler.
func TestInitHandlers(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	catalogMock := svcmocks.NewMockCatalogService(ctrl)
	inventoryMock := svcmocks.NewMockInventoryService(ctrl)
	purchaseMock := svcmocks.NewMockPurchaseService(ctrl)
	pointsMock := svcmocks.NewMockPointsService(ctrl)

	a := New("store")
	a.logger = zap.NewNop()
	a.catalogSvc = catalogMock
	a.inventorySvc = inventoryMock
	a.purchaseSvc = purchaseMock
	a.pointsSvc = pointsMock

	a.initHandlers()

	if a.storeHandler == nil {
		t.Fatal("storeHandler is nil after initHandlers")
	}

	if a.purchaseHandler == nil {
		t.Fatal("purchaseHandler is nil after initHandlers")
	}

	if a.pointsHandler == nil {
		t.Fatal("pointsHandler is nil after initHandlers")
	}
}

// TestInitMiddleware verifies that initMiddleware wires authMiddleware.
func TestInitMiddleware(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	sessionMock := svcmocks.NewMockSessionService(ctrl)

	a := New("store")
	a.logger = zap.NewNop()
	a.cfg = &config.Config{Session: config.SessionConfig{CookieName: "session"}}
	a.sessionSvc = sessionMock

	a.initMiddleware()

	if a.authMiddleware == nil {
		t.Fatal("authMiddleware is nil after initMiddleware")
	}
}

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

func TestRunWorkers_WorkerError(t *testing.T) {
	t.Parallel()

	// A bind address with no port makes e.Start fail, surfacing a worker error.
	a := newTestApp(t, "not-a-valid-bind-addr")

	if err := a.runWorkers(context.Background()); err == nil {
		t.Fatal("runWorkers() error = nil, want the worker's start error")
	}
}

func TestRun_FailsOnConfig(t *testing.T) {
	t.Parallel()

	// No IS_DOCKER -> file mode; cmd/config.yaml does not exist from this package's
	// working directory, so populateConfig (and thus Run) must fail.
	a := New("store")
	if err := a.Run(context.Background()); err == nil {
		t.Fatal("Run() error = nil, want a config-load error")
	}
}

func TestRun_FailsOnPostgres(t *testing.T) {
	t.Setenv("IS_DOCKER", "1")
	t.Setenv("HTTP_ADDR", "127.0.0.1:0")
	t.Setenv("POSTGRES_DSN", "postgres://localhost:not-a-port/db")
	t.Setenv("REDIS_ADDR", "127.0.0.1:6379")
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_key")
	t.Setenv("STRIPE_WEBHOOK_SIGNING_SECRET", "whsec_secret")

	a := New("store")
	if err := a.Run(context.Background()); err == nil {
		t.Fatal("Run() error = nil, want a Postgres connect error")
	}
}

// newTestApp builds an App wired with the minimum needed to exercise the HTTP
// worker and router: a no-op logger, the validator, an HTTP addr, and stub
// handler + middleware so route registration doesn't panic.
func newTestApp(t *testing.T, addr string) *App {
	t.Helper()

	ctrl := gomock.NewController(t)
	catalogMock := svcmocks.NewMockCatalogService(ctrl)
	inventoryMock := svcmocks.NewMockInventoryService(ctrl)
	sessionMock := svcmocks.NewMockSessionService(ctrl)
	purchaseMock := svcmocks.NewMockPurchaseService(ctrl)
	pointsMock := svcmocks.NewMockPointsService(ctrl)

	a := New("store-test")
	a.logger = zap.NewNop()
	a.initValidator()
	a.cfg = &config.Config{
		HTTP:    config.HTTPConfig{Addr: addr},
		Session: config.SessionConfig{CookieName: "session"},
	}

	// Wire stub handlers and middleware so registerHTTPRoutes works.
	a.storeHandler = delivery.NewStoreHandler(catalogMock, inventoryMock, zap.NewNop())
	a.purchaseHandler = delivery.NewPurchaseHandler(purchaseMock, zap.NewNop())
	a.pointsHandler = delivery.NewPointsHandler(pointsMock, zap.NewNop())
	a.authMiddleware = middleware.NewAuthMiddleware(sessionMock, "session", zap.NewNop())

	return a
}
