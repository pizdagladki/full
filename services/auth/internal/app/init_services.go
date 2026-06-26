package app

import (
	"github.com/pizdagladki/full/services/auth/internal/api/service"
)

func (a *App) initServices() {
	a.oauth = service.NewGoogleOAuth(
		a.cfg.GoogleOAuth.ClientID,
		a.cfg.GoogleOAuth.ClientSecret,
		a.cfg.GoogleOAuth.RedirectURL,
	)
	a.sessionStore = service.NewRedisSessionStore(a.redisClient, a.cfg.Session.TTL)
	a.authService = service.NewAuthService(a.userRepo, a.oauth, a.sessionStore, a.logger)
	a.consentService = service.NewConsentService(a.consentRepo, a.logger)
}
