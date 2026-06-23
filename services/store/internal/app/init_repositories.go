package app

import "github.com/pizdagladki/full/services/store/internal/api/repository"

func (a *App) initRepositories() {
	a.catalogRepo = repository.NewCatalogRepository(a.pgxPool)
	a.inventoryRepo = repository.NewInventoryRepository(a.pgxPool)
	a.sessionRepo = repository.NewSessionRepository(a.redisClient)
}
