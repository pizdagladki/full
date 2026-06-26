package app

import (
	"net/http"
	"time"

	"github.com/pizdagladki/full/services/signaling/internal/api/service"
)

func (a *App) initServices() {
	ratingsClient := service.NewHTTPRatingsClient(a.cfg.RatingsBaseURL, &http.Client{Timeout: 10 * time.Second})
	a.signalingSvc = service.NewSignalingService(
		a.logger,
		a.roomRepo,
		time.Now,
		time.AfterFunc,
		a.cfg.Signaling.ConfirmationBuffer,
		ratingsClient,
	)
}
