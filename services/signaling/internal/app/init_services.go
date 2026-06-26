package app

import (
	"github.com/pizdagladki/full/services/signaling/internal/api/service"
)

func (a *App) initServices() {
	a.signalingSvc = service.NewSignalingService(a.logger, a.roomRepo)
}
