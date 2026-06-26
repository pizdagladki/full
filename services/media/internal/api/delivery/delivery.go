// Package delivery holds the media service HTTP handlers (transport layer:
// request parse/validate, status codes, serialization). Handler interfaces are
// added here by downstream resource slices via the new-resource skill; the
// scaffold ships only the liveness probe wired in the app layer.
package delivery

import "github.com/labstack/echo/v4"

// UserIDContextKey is the key under which RequireAuth stores the int64 user ID
// in the Echo context (via c.Set / c.Get). Both the middleware and the handlers
// use this constant to avoid magic strings.
const UserIDContextKey = "media_user_id"

// ClipHandler is the transport contract for the clips resource.
type ClipHandler interface {
	// Upload handles POST /v1/clips.
	Upload(c echo.Context) error
	// List handles GET /v1/clips.
	List(c echo.Context) error
	// Download handles GET /v1/clips/:id/download.
	Download(c echo.Context) error
}
