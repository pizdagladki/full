package app

import (
	"time"

	"github.com/pizdagladki/full/services/store/internal/api/repository"
)

func (a *App) initRepositories() {
	a.catalogRepo = repository.NewCatalogRepository(a.pgxPool)
	a.inventoryRepo = repository.NewInventoryRepository(a.pgxPool)
	a.sessionRepo = repository.NewSessionRepository(a.redisClient)
	a.purchaseRepo = repository.NewPurchaseRepository(a.pgxPool)
	a.pointsRepo = repository.NewPointsRepository(a.pgxPool)
	a.pointsCache = repository.NewPointsCache(a.redisClient)
	a.rewardedRepo = repository.NewRewardedRepository(a.pgxPool)
	a.rewardedLimiter = repository.NewRewardedRateLimiter(
		a.redisClient, a.cfg.Rewarded.Cap, time.Duration(a.cfg.Rewarded.WindowSeconds)*time.Second,
	)
}
