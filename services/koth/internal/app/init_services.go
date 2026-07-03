package app

import (
	"net/http"

	"github.com/pizdagladki/full/services/koth/internal/api/service"
)

func (a *App) initServices() {
	pointsClient := service.NewHTTPPointsClient(a.cfg.Store.BaseURL, &http.Client{Timeout: pointsClientTimeout})

	a.rankSvc = service.NewRankService(
		a.rankRepo, service.RealClock, a.cfg.Ranked.Thresholds, pointsClient, a.cfg.Points.RankAmount, a.logger,
	)
	a.hillSvc = service.NewHillService(a.hillRepo, pointsClient, a.cfg.Points.WinAmount, a.logger)
	a.sessionSvc = service.NewSessionService(a.sessionRepo)
}
