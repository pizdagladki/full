// Package service holds the signaling service business logic (SDP/ICE relay
// and peer-left notification on disconnect).
package service

import "context"

//go:generate mockgen -source=service.go -destination=mocks/service_mock.go -package=mocks

type (
	// Conn is an abstraction over a WebSocket connection that the service
	// layer can use without touching coder/websocket directly. This keeps the
	// service layer fully unit-testable with fake connections.
	Conn interface {
		// UserID returns the authenticated user id for this connection.
		UserID() int64
		// Send writes raw bytes as a single WS frame to the peer.
		Send(raw []byte) error
		// Close sends a close frame and tears down the connection.
		Close(reason string)
	}

	// SignalingService is the primary entry-point for the WS handler.
	SignalingService interface {
		// Join validates roomID, adds the peer to the Redis room set, and
		// registers the connection in the in-process hub.
		// Returns ErrRoomFull when the room already has two members.
		Join(ctx context.Context, conn Conn, roomID string) error

		// Relay forwards raw bytes verbatim to every other member of roomID.
		// The sender must be a registered in-process member; if not, returns
		// ErrNotMember and does not relay.
		Relay(ctx context.Context, conn Conn, roomID string, raw []byte) error

		// Leave is called on disconnect: notifies the peer with peer_left,
		// removes the sender from the in-process hub, closes the peer connection,
		// and deletes the room from Redis.
		Leave(ctx context.Context, conn Conn, roomID string)

		// ReportEvent records a blink/face_lost event from conn in roomID,
		// stamped with the server receive-time (client-sent timestamps are ignored).
		// Returns ErrNotMember if conn is not in the room, or ErrMatchFinished if
		// an outcome was already decided (idempotent).
		ReportEvent(ctx context.Context, conn Conn, roomID string, eventType string) error
	}
)
