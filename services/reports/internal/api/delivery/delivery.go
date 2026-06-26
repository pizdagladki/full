// Package delivery holds the reports service HTTP handlers (transport layer:
// request parse/validate, status codes, serialization). Handler interfaces are
// added here by downstream resource slices via the new-resource skill; the
// scaffold ships only the liveness probe wired in the app layer.
package delivery

import "github.com/labstack/echo/v4"

type (
	// ReportsHandler exposes cheat-report HTTP endpoints.
	ReportsHandler interface {
		// PostCheatReport handles POST /v1/reports/cheat.
		PostCheatReport(c echo.Context) error
		// GetCooldown handles GET /v1/reports/cooldown/:user_id.
		GetCooldown(c echo.Context) error
	}
)
