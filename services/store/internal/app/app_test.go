package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/store/internal/api/delivery"
	"github.com/pizdagladki/full/services/store/internal/api/domain"
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

// TestRegisterHTTPRoutes_InternalAuthAndSessionGuards drives requests through
// the App's REAL router (a.registerHTTPRoutes(), the same one wired in
// production) rather than re-wrapping the middleware by hand. This is what
// actually proves POST /v1/points/credit is registered under the
// internalauth-gated group (not public) and that the session-protected group
// still carries RequireAuth — a route re-registered publicly, or a dropped
// RequireAuth, would flip one of these assertions.
func TestRegisterHTTPRoutes_InternalAuthAndSessionGuards(t *testing.T) {
	t.Parallel()

	const internalToken = "s2s-secret-token"

	ctrl := gomock.NewController(t)
	catalogMock := svcmocks.NewMockCatalogService(ctrl)
	inventoryMock := svcmocks.NewMockInventoryService(ctrl)
	sessionMock := svcmocks.NewMockSessionService(ctrl)
	purchaseMock := svcmocks.NewMockPurchaseService(ctrl)
	pointsMock := svcmocks.NewMockPointsService(ctrl)
	rewardedMock := svcmocks.NewMockRewardedService(ctrl)

	const validSessionCookie = "valid-session-value"

	const sessionUserID = int64(7)

	// Only reached if the request actually makes it past internalauth to the
	// real Credit handler (the correct-token case).
	pointsMock.EXPECT().Credit(gomock.Any(), gomock.Any()).Return(int64(10), nil).AnyTimes()

	// Only reached if RequireAuth is still attached to the session group AND
	// resolves the cookie (the valid-cookie case below); a dropped RequireAuth
	// means these are never invoked and the handler 401s on the missing
	// context value instead of returning 200.
	sessionMock.EXPECT().ResolveSession(gomock.Any(), validSessionCookie).Return(sessionUserID, nil).AnyTimes()
	inventoryMock.EXPECT().ListInventory(gomock.Any(), sessionUserID).Return([]domain.InventoryItem{}, nil).AnyTimes()

	a := New("store-test")
	a.logger = zap.NewNop()
	a.initValidator()
	a.cfg = &config.Config{
		HTTP:     config.HTTPConfig{Addr: "127.0.0.1:0"},
		Session:  config.SessionConfig{CookieName: "session"},
		Internal: config.InternalConfig{APIToken: internalToken},
	}

	a.storeHandler = delivery.NewStoreHandler(catalogMock, inventoryMock, zap.NewNop())
	a.purchaseHandler = delivery.NewPurchaseHandler(purchaseMock, zap.NewNop())
	a.pointsHandler = delivery.NewPointsHandler(pointsMock, zap.NewNop())
	a.rewardedHandler = delivery.NewRewardedHandler(rewardedMock, zap.NewNop())
	a.authMiddleware = middleware.NewAuthMiddleware(sessionMock, "session", zap.NewNop())

	e := a.registerHTTPRoutes()

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		authHeader string
		cookie     string
		wantStatus int
	}{
		{
			// criterion 3: the credit route must NOT be public. If someone
			// re-registered POST /v1/points/credit outside the internalauth
			// group, this request (no Authorization header) would reach the
			// handler and return 200 instead of 401, failing this case.
			name:       "credit route with no internal token -> 401 (not public)",
			method:     http.MethodPost,
			path:       "/v1/points/credit",
			body:       `{"user_id":1,"reason":"match_win","ref_id":"m-1"}`,
			wantStatus: http.StatusUnauthorized,
		},
		{
			// criterion 3: the credit route IS reachable through the router
			// with the correct internal bearer token, proving it is wired to
			// the internalauth-gated group and to the real handler.
			name:       "credit route with correct internal token -> 200 (reaches guarded handler)",
			method:     http.MethodPost,
			path:       "/v1/points/credit",
			body:       `{"user_id":1,"reason":"match_win","ref_id":"m-1"}`,
			authHeader: "Bearer " + internalToken,
			wantStatus: http.StatusOK,
		},
		{
			// criterion 3: the session-protected group is unchanged — a
			// session route with no cookie still 401s (either via RequireAuth
			// or the handler's own defense-in-depth check).
			name:       "session-protected inventory route with no cookie -> 401",
			method:     http.MethodGet,
			path:       "/v1/store/inventory",
			wantStatus: http.StatusUnauthorized,
		},
		{
			// criterion 3: RequireAuth is still attached to the session
			// group — a VALID session cookie must resolve to a user and reach
			// the handler (200). If RequireAuth were dropped from the /v1
			// group, ResolveSession would never run, the handler would never
			// see a user id in context, and this would 401 instead of 200.
			name:       "session-protected inventory route with valid cookie -> 200 (RequireAuth intact)",
			method:     http.MethodGet,
			path:       "/v1/store/inventory",
			cookie:     validSessionCookie,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body *strings.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			} else {
				body = strings.NewReader("")
			}

			req := httptest.NewRequest(tt.method, tt.path, body)
			if tt.body != "" {
				req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			}
			if tt.authHeader != "" {
				req.Header.Set(echo.HeaderAuthorization, tt.authHeader)
			}
			if tt.cookie != "" {
				req.AddCookie(&http.Cookie{Name: "session", Value: tt.cookie})
			}
			rec := httptest.NewRecorder()

			e.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d, body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
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
	// they never dereference it during construction. cfg must be non-nil —
	// initRepositories reads cfg.Rewarded to build the rate limiter.
	a.cfg = &config.Config{}
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
	if a.rewardedRepo == nil {
		t.Fatal("rewardedRepo is nil after initRepositories")
	}
	if a.rewardedLimiter == nil {
		t.Fatal("rewardedLimiter is nil after initRepositories")
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
	if a.rewardedSvc == nil {
		t.Error("rewardedSvc is nil after initServices")
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
	rewardedMock := svcmocks.NewMockRewardedService(ctrl)

	a := New("store")
	a.logger = zap.NewNop()
	a.catalogSvc = catalogMock
	a.inventorySvc = inventoryMock
	a.purchaseSvc = purchaseMock
	a.pointsSvc = pointsMock
	a.rewardedSvc = rewardedMock

	a.initHandlers()

	if a.storeHandler == nil {
		t.Fatal("storeHandler is nil after initHandlers")
	}

	if a.rewardedHandler == nil {
		t.Fatal("rewardedHandler is nil after initHandlers")
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
	rewardedMock := svcmocks.NewMockRewardedService(ctrl)

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
	a.rewardedHandler = delivery.NewRewardedHandler(rewardedMock, zap.NewNop())
	a.authMiddleware = middleware.NewAuthMiddleware(sessionMock, "session", zap.NewNop())

	return a
}
