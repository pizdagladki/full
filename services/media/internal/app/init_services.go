package app

import (
	"github.com/pizdagladki/full/services/media/internal/api/service"
)

func (a *App) initServices() {
	a.clipSvc = service.NewClipService(
		a.clipRepo,
		a.objectStore,
		service.ClipServiceConfig{
			MaxUploadBytes: a.cfg.Clips.MaxUploadBytes,
			DownloadURLTTL: a.cfg.Clips.DownloadURLTTL,
		},
		a.logger,
	)
	a.sessionSvc = service.NewSessionService(a.sessionRepo)
}
