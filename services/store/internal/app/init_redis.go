package app

import (
	"context"

	"go.uber.org/zap"

	platformredis "github.com/pizdagladki/full/internal/platform/redis"
)

func (a *App) initRedis(ctx context.Context) error {
	client, err := platformredis.New(ctx, a.cfg.Redis.Addr, a.cfg.Redis.Password)
	if err != nil {
		a.logger.Error("connect redis", zap.Error(err))

		return err
	}

	a.redisClient = client

	return nil
}
