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

	// Protected clip endpoints — session required.
	v1 := e.Group("/v1", a.authMiddleware.RequireAuth)
	v1.POST("/clips", a.clipHandler.Upload)
	v1.GET("/clips", a.clipHandler.List)
	v1.GET("/clips/:id/download", a.clipHandler.Download)
	v1.POST("/clips/:id/convert", a.clipHandler.Convert)
	v1.GET("/clips/:id/mp4", a.clipHandler.GetMP4)

	return e
}

// handleHealthz is the liveness probe: it reports that the process is up.
func handleHealthz(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
