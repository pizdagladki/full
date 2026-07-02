package app

import "github.com/pizdagladki/full/services/koth/internal/api/service"

func (a *App) initServices() {
	a.rankSvc = service.NewRankService(a.rankRepo, service.RealClock, a.cfg.Ranked.Thresholds)
	a.sessionSvc = service.NewSessionService(a.sessionRepo)
}
