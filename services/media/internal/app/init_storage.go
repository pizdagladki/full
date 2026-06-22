package app

import (
	"context"

	"go.uber.org/zap"

	platformstorage "github.com/pizdagladki/full/internal/platform/storage"
)

func (a *App) initStorage(ctx context.Context) error {
	client, err := platformstorage.New(ctx, platformstorage.Config{
		Endpoint:  a.cfg.Storage.Endpoint,
		AccessKey: a.cfg.Storage.AccessKey,
		SecretKey: a.cfg.Storage.SecretKey,
		Bucket:    a.cfg.Storage.Bucket,
		UseSSL:    a.cfg.Storage.UseSSL,
	})
	if err != nil {
		a.logger.Error("connect minio", zap.Error(err))

		return err
	}

	a.minioClient = client

	return nil
}
