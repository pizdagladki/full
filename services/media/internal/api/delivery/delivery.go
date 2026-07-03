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
	// Convert handles POST /v1/clips/:id/convert.
	Convert(c echo.Context) error
	// GetMP4 handles GET /v1/clips/:id/mp4.
	GetMP4(c echo.Context) error
}

// KingClipHandler is the transport contract for the king-of-the-hill clips
// resource. King clips are a category separate from clips (win-clips): their
// own routes, own storage prefix, no keep-last-10 FIFO.
type KingClipHandler interface {
	// Upload handles POST /v1/king-clips.
	Upload(c echo.Context) error
	// Current handles GET /v1/king-clips/current.
	Current(c echo.Context) error
	// Delete handles DELETE /v1/king-clips/:id.
	Delete(c echo.Context) error
}
