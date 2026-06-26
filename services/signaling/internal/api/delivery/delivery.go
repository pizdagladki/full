// Package delivery holds the signaling service WebSocket and HTTP handlers
// (transport layer: request parse/validate, status codes, serialization).
package delivery

import "net/http"

type (
	// SignalingHandler handles the /ws WebSocket endpoint for SDP/ICE relay.
	SignalingHandler interface {
		ServeWS(w http.ResponseWriter, r *http.Request)
	}
)
