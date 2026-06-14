// Package app assembles and runs the health service.
package app

import (
	"context"

	"github.com/pizdagladki/full/internal/platform/logger"
	"github.com/pizdagladki/full/services/health/internal/api/delivery"
	"github.com/pizdagladki/full/services/health/internal/api/service"
	"github.com/pizdagladki/full/services/health/internal/config"
	"go.uber.org/zap"
)

// App holds the service dependencies and drives its lifecycle.
type App struct {
	name string

	logger *zap.Logger
	cfg    *config.Config

	healthService service.HealthService
	healthHandler delivery.HealthHandler
}

// New returns an empty App for the given service name.
func New(name string) *App {
	return &App{name: name}
}

// Run initializes dependencies in order and runs the workers until ctx is
// canceled (graceful shutdown).
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

func (a *App) initServices() {
	a.healthService = service.NewHealthService()
}

func (a *App) initHandlers() {
	a.healthHandler = delivery.NewHealthHandler(a.healthService, a.logger)
}
