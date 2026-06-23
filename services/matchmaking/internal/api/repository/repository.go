// Package repository holds the matchmaking service data access (Redis-backed
// queue and pairing state).
package repository

import (
	"context"
	"errors"

	"github.com/pizdagladki/full/services/matchmaking/internal/api/domain"
)

// ErrSessionNotFound is returned when the session key is absent or expired.
var ErrSessionNotFound = errors.New("session not found")

//go:generate mockgen -source=repository.go -destination=mocks/repository_mock.go -package=mocks

type (
	// QueueRepository manages the matchmaking queue in Redis.
	QueueRepository interface {
		// Enqueue adds player to the waiting queue for their mode.
		Enqueue(ctx context.Context, player domain.Player) error
		// ListWaiting returns all players waiting in the given mode.
		ListWaiting(ctx context.Context, mode string) ([]domain.Player, error)
		// Remove deletes player from the queue. Returns true iff this call
		// actually removed the entry (false if it was already gone).
		Remove(ctx context.Context, mode string, userID int64) (bool, error)
		// Pair atomically removes both a and b from the queue, succeeding only
		// when BOTH entries are present at call time. Returns false if either
		// is already gone (lost the race).
		Pair(ctx context.Context, a, b domain.Player) (bool, error)
		// Refresh resets the backstop TTL on the queue hash for the given mode.
		// Call this each Tick while a mode still has live waiters so connected
		// players are never evicted by the orphan-cleanup TTL.
		Refresh(ctx context.Context, mode string) error
	}

	// SessionRepository reads read-only session data from Redis.
	SessionRepository interface {
		// UserIDBySession resolves a session cookie value to a user id.
		// Returns ErrSessionNotFound on redis.Nil (absent or expired).
		UserIDBySession(ctx context.Context, sessionID string) (int64, error)
	}
)
