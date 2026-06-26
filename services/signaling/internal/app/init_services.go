package app

import (
	"time"

	"github.com/pizdagladki/full/services/signaling/internal/api/service"
)

func (a *App) initServices() {
	a.signalingSvc = service.NewSignalingService(
		a.logger,
		a.roomRepo,
		time.Now,
		time.AfterFunc,
		a.cfg.Signaling.ConfirmationBuffer,
	)
}
