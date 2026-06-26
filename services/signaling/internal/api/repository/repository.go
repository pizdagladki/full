// Package repository holds the signaling service data access (Redis-backed
// session and room state).
package repository

import (
	"context"
	"errors"
)

// ErrSessionNotFound is returned when the session key is absent or expired.
var ErrSessionNotFound = errors.New("session not found")

//go:generate mockgen -source=repository.go -destination=mocks/repository_mock.go -package=mocks

// JoinResult encodes the three outcomes of a Join attempt.
type JoinResult int

const (
	// JoinResultJoined means the caller was successfully added to the room.
	JoinResultJoined JoinResult = iota
	// JoinResultAlreadyMember means the caller was already in the room (idempotent).
	JoinResultAlreadyMember
	// JoinResultFull means the room already has two members; the caller was not added.
	JoinResultFull
)

type (
	// SessionRepository reads read-only session data from Redis.
	SessionRepository interface {
		// UserIDBySession resolves a session cookie value to a user id.
		// Returns ErrSessionNotFound on redis.Nil (absent or expired).
		UserIDBySession(ctx context.Context, sessionID string) (int64, error)
	}

	// RoomRepository manages WebRTC room membership in Redis.
	RoomRepository interface {
		// Join atomically tries to add userID to roomID.
		// Returns JoinResultJoined (new member), JoinResultAlreadyMember (idempotent),
		// or JoinResultFull when the room already holds two distinct members.
		Join(ctx context.Context, roomID string, userID int64) (JoinResult, error)
		// IsMember reports whether userID is a member of roomID.
		IsMember(ctx context.Context, roomID string, userID int64) (bool, error)
		// RemoveRoom deletes the room key from Redis (called on peer disconnect).
		RemoveRoom(ctx context.Context, roomID string) error
	}
)
