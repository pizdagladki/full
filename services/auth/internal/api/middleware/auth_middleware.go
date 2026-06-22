// Package middleware holds the auth service cross-cutting request logic. The
// RequireAuth middleware (Redis-session validation) is added here by the auth
// login slice via the new-resource skill.
package middleware

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/auth/internal/api/delivery"
	"github.com/pizdagladki/full/services/auth/internal/api/service"
)

// AuthMiddleware validates incoming requests by looking up the session cookie in
// the Redis session store.
type AuthMiddleware struct {
	svc        service.AuthService
	cookieName string
	logger     *zap.Logger
}

// NewAuthMiddleware constructs an AuthMiddleware.
func NewAuthMiddleware(svc service.AuthService, cookieName string, logger *zap.Logger) *AuthMiddleware {
	return &AuthMiddleware{svc: svc, cookieName: cookieName, logger: logger}
}

// RequireAuth is an echo.MiddlewareFunc that reads the session cookie, validates
// it via AuthService.Authenticate, and stores the resolved domain.User in the
// echo context under delivery.UserContextKey.
func (m *AuthMiddleware) RequireAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		cookie, err := c.Cookie(m.cookieName)
		if err != nil {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		}

		user, err := m.svc.Authenticate(c.Request().Context(), cookie.Value)
		if err != nil {
			if errors.Is(err, service.ErrSessionNotFound) {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "session expired or not found"})
			}

			m.logger.Warn("authenticate session", zap.Error(err))

			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		}

		c.Set(delivery.UserContextKey, user)

		return next(c)
	}
}
