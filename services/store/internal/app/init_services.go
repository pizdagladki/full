package app

import "github.com/pizdagladki/full/services/store/internal/api/service"

func (a *App) initServices() {
	a.catalogSvc = service.NewCatalogService(a.catalogRepo)
	a.inventorySvc = service.NewInventoryService(a.inventoryRepo)
	a.sessionSvc = service.NewSessionService(a.sessionRepo)
}
