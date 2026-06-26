package app

import (
	"github.com/pizdagladki/full/services/signaling/internal/api/delivery"
)

func (a *App) initHandlers() {
	a.wsHandler = delivery.NewSignalingHandler(
		a.logger,
		a.sessionRepo,
		a.signalingSvc,
		a.cfg.Signaling.SessionCookie,
		a.cfg.Signaling.KeepaliveInterval,
		a.cfg.Signaling.KeepalivePingTimeout,
	)
}
