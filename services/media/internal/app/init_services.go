package app

import (
	"fmt"
	"os/exec"

	svc "github.com/pizdagladki/full/services/media/internal/api/service"
)

func (a *App) initServices() error {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("ffmpeg binary not found: %w", err)
	}

	runner := svc.NewExecFFmpegRunner(bin)

	a.clipSvc = svc.NewClipService(
		a.clipRepo,
		a.objectStore,
		runner,
		svc.ClipServiceConfig{
			MaxUploadBytes: a.cfg.Clips.MaxUploadBytes,
			DownloadURLTTL: a.cfg.Clips.DownloadURLTTL,
		},
		a.logger,
	)
	a.sessionSvc = svc.NewSessionService(a.sessionRepo)

	return nil
}
