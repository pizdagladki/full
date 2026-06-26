package app

import "github.com/pizdagladki/full/services/store/internal/api/delivery"

func (a *App) initHandlers() {
	a.storeHandler = delivery.NewStoreHandler(a.catalogSvc, a.inventorySvc, a.logger)
	a.purchaseHandler = delivery.NewPurchaseHandler(a.purchaseSvc, a.logger)
}
