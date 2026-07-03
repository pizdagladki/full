package app

import (
	"github.com/pizdagladki/full/services/media/internal/api/repository"
)

func (a *App) initRepositories() {
	a.clipRepo = repository.NewClipRepository(a.pgxPool)
	a.sessionRepo = repository.NewSessionRepository(a.redisClient)
	a.kingClipRepo = repository.NewKingClipRepository(a.pgxPool)
	a.objectStore = repository.NewMinioObjectStore(a.minioClient, a.cfg.Storage.Bucket)
}
