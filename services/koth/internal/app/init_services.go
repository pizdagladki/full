package app

import (
	"net/http"

	"github.com/pizdagladki/full/services/koth/internal/api/service"
)

func (a *App) initServices() {
	pointsClient := service.NewHTTPPointsClient(
		a.cfg.Store.BaseURL, a.cfg.Internal.APIToken, &http.Client{Timeout: pointsClientTimeout},
	)
	mediaClient := service.NewHTTPMediaClient(
		a.cfg.Media.BaseURL, a.cfg.Internal.APIToken, &http.Client{Timeout: mediaClientTimeout},
	)

	a.rankSvc = service.NewRankService(
		a.rankRepo, service.RealClock, a.cfg.Ranked.Thresholds, pointsClient, a.cfg.Points.RankAmount, a.logger,
	)
	a.hillSvc = service.NewHillService(a.hillRepo, pointsClient, a.cfg.Points.WinAmount, a.logger)
	a.sessionSvc = service.NewSessionService(a.sessionRepo)
	a.resetSvc = service.NewResetService(a.hillRepo, service.RealClock, pointsClient, mediaClient, a.logger)
}
