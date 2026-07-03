package app

import (
	"net/http"
	"time"

	"github.com/pizdagladki/full/services/koth/internal/api/service"
)

// pointsClientTimeout bounds a single points-credit HTTP call to the store.
const pointsClientTimeout = 10 * time.Second

// mediaClientTimeout bounds a single king-clip-expiry HTTP call to media.
const mediaClientTimeout = 10 * time.Second

func (a *App) initServices() {
	a.rankSvc = service.NewRankService(a.rankRepo, service.RealClock, a.cfg.Ranked.Thresholds)
	a.hillSvc = service.NewHillService(a.hillRepo)
	a.sessionSvc = service.NewSessionService(a.sessionRepo)

	pointsClient := service.NewHTTPPointsClient(a.cfg.Store.BaseURL, &http.Client{Timeout: pointsClientTimeout})
	mediaClient := service.NewHTTPMediaClient(a.cfg.Media.BaseURL, &http.Client{Timeout: mediaClientTimeout})
	a.resetSvc = service.NewResetService(a.hillRepo, service.RealClock, pointsClient, mediaClient, a.logger)
}
