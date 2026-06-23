package app

import (
	"github.com/pizdagladki/full/services/media/internal/api/middleware"
)

func (a *App) initMiddleware() {
	a.authMiddleware = middleware.NewAuthMiddleware(a.sessionSvc, a.cfg.Session.CookieName, a.logger)
}
