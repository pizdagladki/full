// Package app assembles and runs the store service.
package app

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/internal/platform/logger"
	"github.com/pizdagladki/full/services/store/internal/api/delivery"
	"github.com/pizdagladki/full/services/store/internal/api/middleware"
	"github.com/pizdagladki/full/services/store/internal/api/repository"
	"github.com/pizdagladki/full/services/store/internal/api/service"
	"github.com/pizdagladki/full/services/store/internal/config"
)

// App holds the service dependencies and drives its lifecycle.
type App struct {
	name string

	logger    *zap.Logger
	validator echo.Validator
	cfg       *config.Config

	pgxPool     *pgxpool.Pool
	redisClient *redis.Client

	catalogRepo   repository.CatalogRepository
	inventoryRepo repository.InventoryRepository
	sessionRepo   repository.SessionRepository
	purchaseRepo  repository.PurchaseRepository

	catalogSvc   service.CatalogService
	inventorySvc service.InventoryService
	sessionSvc   service.SessionService
	purchaseSvc  service.PurchaseService

	paymentProvider service.PaymentProvider

	storeHandler    delivery.StoreHandler
	purchaseHandler delivery.PurchaseHandler
	authMiddleware  *middleware.AuthMiddleware
}

// New returns an empty App for the given service name.
func New(name string) *App {
	return &App{name: name}
}

// Run initializes dependencies in order and runs the workers until ctx is
// canceled (graceful shutdown). A failed Postgres or Redis ping aborts startup.
func (a *App) Run(ctx context.Context) error {
	err := a.initLogger()
	if err != nil {
		return err
	}
	defer func() { _ = a.logger.Sync() }()

	a.logger.Info("starting service", zap.String("service", a.name))

	a.initValidator()

	err = a.populateConfig()
	if err != nil {
		return err
	}

	err = a.initPostgres(ctx)
	if err != nil {
		return err
	}
	defer a.pgxPool.Close()

	err = a.initRedis(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = a.redisClient.Close() }()

	a.initRepositories()
	a.initServices()
	a.initHandlers()
	a.initMiddleware()

	return a.runWorkers(ctx)
}

func (a *App) initLogger() error {
	l, err := logger.New()
	if err != nil {
		return err
	}

	a.logger = l

	return nil
}

func (a *App) populateConfig() error {
	cfg, err := config.Load("cmd/config.yaml")
	if err != nil {
		return err
	}

	a.cfg = cfg

	return nil
}
