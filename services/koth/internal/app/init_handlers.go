package app

import "github.com/pizdagladki/full/services/koth/internal/api/delivery"

func (a *App) initHandlers() {
	a.rankHandler = delivery.NewRankHandler(a.rankSvc, a.logger)
}
