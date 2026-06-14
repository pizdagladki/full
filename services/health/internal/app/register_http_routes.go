package app

import "github.com/labstack/echo/v4"

// registerHTTPRoutes builds the service's Echo router.
func (a *App) registerHTTPRoutes() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.GET("/v1/health", a.healthHandler.Get)

	return e
}
