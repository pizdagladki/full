// Package app assembles and runs the reports service.
package app

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/internal/platform/logger"
	"github.com/pizdagladki/full/services/reports/internal/api/delivery"
	"github.com/pizdagladki/full/services/reports/internal/api/repository"
	"github.com/pizdagladki/full/services/reports/internal/api/service"
	"github.com/pizdagladki/full/services/reports/internal/config"
)

const defaultCooldownTTL = 1800 * time.Second

// App holds the service dependencies and drives its lifecycle.
type App struct {
	name string

	logger    *zap.Logger
	validator echo.Validator
	cfg       *config.Config

	pgxPool     *pgxpool.Pool
	redisClient *redis.Client

	cheatRepo      repository.CheatReportsRepository
	cooldownStore  repository.CooldownStore
	reportsService service.ReportsService
	reportsHandler delivery.ReportsHandler
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

func (a *App) initRepositories() {
	a.cheatRepo = repository.NewCheatReportsRepository(a.pgxPool)
	a.cooldownStore = repository.NewCooldownStore(a.redisClient)
}

func (a *App) initServices() {
	ttl := defaultCooldownTTL
	if a.cfg.Reports.CooldownTTLSeconds > 0 {
		ttl = time.Duration(a.cfg.Reports.CooldownTTLSeconds) * time.Second
	}

	a.reportsService = service.NewReportsService(a.cheatRepo, a.cooldownStore, a.logger, ttl)
}

func (a *App) initHandlers() {
	a.reportsHandler = delivery.NewReportsHandler(a.reportsService, a.logger)
}
