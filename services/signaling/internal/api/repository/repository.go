// Package repository holds the signaling service data access (Redis-backed
// session and room state).
package repository

import (
	"context"
	"errors"
)

// ErrSessionNotFound is returned when the session key is absent or expired.
var ErrSessionNotFound = errors.New("session not found")

// ErrCodeNotFound is returned when an invite code is absent or expired.
var ErrCodeNotFound = errors.New("code not found")

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

	// RoomCodeRepository manages the short invite-code → room_id mapping in
	// Redis for private (invite-a-friend) rooms.
	RoomCodeRepository interface {
		// CreateCode generates a short unique invite code, stores code→roomID in
		// Redis with the code TTL, and returns the code. Retries on collision.
		CreateCode(ctx context.Context, roomID string) (string, error)
		// ResolveCode returns the roomID for a code, or ErrCodeNotFound if absent/expired.
		ResolveCode(ctx context.Context, code string) (string, error)
		// RemoveCode deletes the code→room mapping (called on creator cleanup).
		RemoveCode(ctx context.Context, code string) error
	}
)
