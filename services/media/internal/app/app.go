// Package app assembles and runs the media service.
package app

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/minio/minio-go/v7"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/internal/platform/logger"
	"github.com/pizdagladki/full/services/media/internal/api/delivery"
	"github.com/pizdagladki/full/services/media/internal/api/middleware"
	"github.com/pizdagladki/full/services/media/internal/api/repository"
	"github.com/pizdagladki/full/services/media/internal/api/service"
	"github.com/pizdagladki/full/services/media/internal/config"
)

// App holds the service dependencies and drives its lifecycle.
type App struct {
	name string

	logger    *zap.Logger
	validator echo.Validator
	cfg       *config.Config

	pgxPool     *pgxpool.Pool
	minioClient *minio.Client
	redisClient *redis.Client

	clipRepo     repository.ClipRepository
	sessionRepo  repository.SessionRepository
	kingClipRepo repository.KingClipRepository
	objectStore  service.ObjectStore

	clipSvc     service.ClipService
	sessionSvc  service.SessionService
	kingClipSvc service.KingClipService

	clipHandler     delivery.ClipHandler
	kingClipHandler delivery.KingClipHandler
	authMiddleware  *middleware.AuthMiddleware
}

// New returns an empty App for the given service name.
func New(name string) *App {
	return &App{name: name}
}

// Run initializes dependencies in order and runs the workers until ctx is
// canceled (graceful shutdown). A failed Postgres, MinIO, or Redis connection
// aborts startup.
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

	err = a.initStorage(ctx)
	if err != nil {
		return err
	}

	err = a.initRedis(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = a.redisClient.Close() }()

	a.initRepositories()

	err = a.initServices()
	if err != nil {
		return err
	}

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
