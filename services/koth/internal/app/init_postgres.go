package app

import (
	"context"

	"go.uber.org/zap"

	"github.com/pizdagladki/full/internal/platform/postgres"
)

func (a *App) initPostgres(ctx context.Context) error {
	pool, err := postgres.New(ctx, a.cfg.Postgres.DSN)
	if err != nil {
		a.logger.Error("connect postgres", zap.Error(err))

		return err
	}

	a.pgxPool = pool

	return nil
}
