// Package service holds the matchmaking service business logic (queue
// management, pairing, and connection hub).
package service

import (
	"context"
	"time"

	"github.com/pizdagladki/full/services/matchmaking/internal/api/domain"
)

//go:generate mockgen -source=service.go -destination=mocks/service_mock.go -package=mocks

type (
	// MatchmakingService is the primary entry-point for the WS handler.
	MatchmakingService interface {
		// Join validates the request, enqueues the player, and registers the
		// connection in the hub so the matcher can push results to it.
		Join(ctx context.Context, conn Conn, player domain.Player) error
		// Leave removes the player from the queue and deregisters the connection.
		Leave(ctx context.Context, userID int64, mode string)
		// Tick drives one pairing cycle for all active modes. In production,
		// this is called on a timer; in tests it is called explicitly.
		Tick(ctx context.Context)
	}

	// Conn is an abstraction over a WebSocket connection that the hub/loop
	// can use without touching coder/websocket directly. This keeps the
	// service layer fully unit-testable with fake connections.
	Conn interface {
		// UserID returns the authenticated user id for this connection.
		UserID() int64
		// Send writes a MatchedMessage to the connection.
		Send(msg domain.MatchedMessage) error
		// Close sends a close frame and tears down the connection.
		Close(reason string)
	}

	// Clock is an injectable time source so tests can control wall time.
	Clock interface {
		// Now returns the current time.
		Now() time.Time
	}

	// RoomIDGenerator produces unique room identifiers.
	RoomIDGenerator interface {
		// NewRoomID returns a new unique room id string.
		NewRoomID() string
	}
)
