package app

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// registerHTTPRoutes builds the service's Echo router. Public routes are
// registered directly on the root; protected routes are grouped behind
// RequireAuth.
func (a *App) registerHTTPRoutes() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Validator = a.validator

	e.GET("/healthz", handleHealthz)

	// Public cheat-report and cooldown endpoints (no auth required).
	e.POST("/v1/reports/cheat", a.reportsHandler.PostCheatReport)
	e.GET("/v1/reports/cooldown/:user_id", a.reportsHandler.GetCooldown)

	// Protected bug-report endpoint — requires a valid session cookie.
	v1 := e.Group("/v1", a.authMiddleware.RequireAuth)
	v1.POST("/reports/bug", a.reportsHandler.PostBugReport)

	return e
}

// handleHealthz is the liveness probe: it reports that the process is up.
func handleHealthz(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
