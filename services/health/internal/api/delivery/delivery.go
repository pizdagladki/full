// Package delivery holds the health service HTTP handlers.
package delivery

import "github.com/labstack/echo/v4"

type (
	// HealthHandler serves the health endpoint.
	HealthHandler interface {
		Get(c echo.Context) error
	}
)
