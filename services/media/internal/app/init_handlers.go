package app

import (
	"github.com/pizdagladki/full/services/media/internal/api/delivery"
)

func (a *App) initHandlers() {
	a.clipHandler = delivery.NewClipHandler(a.clipSvc, a.cfg.Clips.MaxUploadBytes, a.logger)
	a.kingClipHandler = delivery.NewKingClipHandler(a.kingClipSvc, a.cfg.Clips.MaxUploadBytes, a.logger)
}
