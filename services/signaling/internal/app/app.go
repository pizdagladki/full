// Package app assembles and runs the signaling service.
package app

import (
	"context"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/internal/platform/logger"
	"github.com/pizdagladki/full/services/signaling/internal/api/delivery"
	"github.com/pizdagladki/full/services/signaling/internal/api/repository"
	"github.com/pizdagladki/full/services/signaling/internal/api/service"
	"github.com/pizdagladki/full/services/signaling/internal/config"
)

// App holds the service dependencies and drives its lifecycle.
type App struct {
	name string

	logger *zap.Logger
	cfg    *config.Config

	redisClient *redis.Client

	sessionRepo  repository.SessionRepository
	roomRepo     repository.RoomRepository
	roomCodeRepo repository.RoomCodeRepository

	signalingSvc service.SignalingService

	wsHandler delivery.SignalingHandler
}

// New returns an empty App for the given service name.
func New(name string) *App {
	return &App{name: name}
}

// Run initializes dependencies in order and runs the workers until ctx is
// canceled (graceful shutdown). A failed Redis ping aborts startup.
func (a *App) Run(ctx context.Context) error {
	err := a.initLogger()
	if err != nil {
		return err
	}
	defer func() { _ = a.logger.Sync() }()

	a.logger.Info("starting service", zap.String("service", a.name))

	err = a.populateConfig()
	if err != nil {
		return err
	}

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
