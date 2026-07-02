// Package middleware holds the koth service cross-cutting request logic. The
// RequireAuth middleware (Redis-session validation) validates a session cookie
// and stores the resolved user_id in the Echo context.
package middleware

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/koth/internal/api/delivery"
	"github.com/pizdagladki/full/services/koth/internal/api/repository"
	"github.com/pizdagladki/full/services/koth/internal/api/service"
)

// AuthMiddleware validates incoming requests by resolving the session cookie
// to a user_id stored in Redis.
type AuthMiddleware struct {
	sessionSvc service.SessionService
	cookieName string
	logger     *zap.Logger
}

// NewAuthMiddleware constructs an AuthMiddleware.
func NewAuthMiddleware(sessionSvc service.SessionService, cookieName string, logger *zap.Logger) *AuthMiddleware {
	return &AuthMiddleware{sessionSvc: sessionSvc, cookieName: cookieName, logger: logger}
}

// RequireAuth is an echo.MiddlewareFunc that reads the session cookie, resolves
// it via the SessionService, and stores the resolved user_id (int64) in the
// echo context under delivery.UserIDContextKey.
// Missing cookie, redis.Nil, or parse errors all return 401.
func (m *AuthMiddleware) RequireAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		cookie, err := c.Cookie(m.cookieName)
		if err != nil {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		}

		userID, err := m.sessionSvc.ResolveSession(c.Request().Context(), cookie.Value)
		if err != nil {
			if errors.Is(err, repository.ErrSessionNotFound) {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			}

			m.logger.Warn("resolve session", zap.Error(err))

			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		}

		c.Set(delivery.UserIDContextKey, userID)

		return next(c)
	}
}
