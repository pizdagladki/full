package app

import "github.com/pizdagladki/full/services/store/internal/api/delivery"

func (a *App) initHandlers() {
	a.storeHandler = delivery.NewStoreHandler(a.catalogSvc, a.inventorySvc, a.logger)
	a.purchaseHandler = delivery.NewPurchaseHandler(a.purchaseSvc, a.logger)
	a.pointsHandler = delivery.NewPointsHandler(a.pointsSvc, a.logger)
	a.rewardedHandler = delivery.NewRewardedHandler(a.rewardedSvc, a.logger)
}
