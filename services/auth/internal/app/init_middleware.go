package app

import "github.com/pizdagladki/full/services/auth/internal/api/middleware"

func (a *App) initMiddleware() {
	a.authMiddleware = middleware.NewAuthMiddleware(a.authService, a.cfg.Session.Name, a.logger)
}
