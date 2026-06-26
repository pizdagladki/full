package app

import (
	"github.com/pizdagladki/full/services/auth/internal/api/delivery"
)

func (a *App) initHandlers() {
	a.authHandler = delivery.NewAuthHandler(a.authService, a.consentService, a.logger, delivery.HandlerConfig{
		CookieName:   a.cfg.Session.Name,
		CookieTTL:    a.cfg.Session.TTL,
		CookieSecure: a.cfg.Session.Secure,
	})
}
