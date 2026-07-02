package app

import "github.com/pizdagladki/full/services/koth/internal/api/middleware"

func (a *App) initMiddleware() {
	cookieName := a.cfg.Session.CookieName
	if cookieName == "" {
		cookieName = "session"
	}

	a.authMiddleware = middleware.NewAuthMiddleware(a.sessionSvc, cookieName, a.logger)
}
