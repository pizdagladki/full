// Package delivery holds the matchmaking service WebSocket handler (transport
// layer: request parse/validate, status codes, serialization).
package delivery

import (
	"net/http"
)

// MatchmakingHandler handles the authenticated /ws endpoint.
type MatchmakingHandler interface {
	// ServeWS handles an incoming WebSocket upgrade request.
	ServeWS(w http.ResponseWriter, r *http.Request)
}
