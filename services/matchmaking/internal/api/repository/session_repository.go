package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
)

// sessionRepository is the Redis-backed SessionRepository implementation.
type sessionRepository struct {
	client *redis.Client
}

// NewSessionRepository returns a SessionRepository backed by the given Redis client.
func NewSessionRepository(client *redis.Client) SessionRepository {
	return &sessionRepository{client: client}
}

// UserIDBySession reads the session key "session:<sessionID>" from Redis and
// parses its value as an int64 user id. Returns ErrSessionNotFound on
// redis.Nil (absent or expired key).
func (r *sessionRepository) UserIDBySession(ctx context.Context, sessionID string) (int64, error) {
	key := "session:" + sessionID

	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, ErrSessionNotFound
		}

		return 0, fmt.Errorf("get session %q: %w", key, err)
	}

	userID, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse user id from session %q: %w", key, err)
	}

	return userID, nil
}
