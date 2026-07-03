package app

import "github.com/pizdagladki/full/services/store/internal/api/service"

func (a *App) initServices() {
	a.catalogSvc = service.NewCatalogService(a.catalogRepo)
	a.inventorySvc = service.NewInventoryService(a.inventoryRepo)
	a.sessionSvc = service.NewSessionService(a.sessionRepo)
	a.paymentProvider = service.NewStripePaymentProvider(a.cfg.Stripe.SecretKey, a.cfg.Stripe.WebhookSigningSecret)
	a.purchaseSvc = service.NewPurchaseService(a.purchaseRepo, a.paymentProvider, a.pointsCache, a.logger)
	a.pointsSvc = service.NewPointsService(a.pointsRepo, a.pointsCache, a.cfg.Points.Amounts, a.logger)
}
