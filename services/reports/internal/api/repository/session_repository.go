package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
)

const sessionKeyPrefix = "session:"

type sessionRepository struct {
	client *redis.Client
}

// NewSessionRepository constructs a Redis-backed SessionRepository.
func NewSessionRepository(client *redis.Client) SessionRepository {
	return &sessionRepository{client: client}
}

// UserIDBySession looks up the user ID stored at session:<sessionID>.
// Returns ErrSessionNotFound when the key is absent or expired.
func (r *sessionRepository) UserIDBySession(ctx context.Context, sessionID string) (int64, error) {
	key := sessionKeyPrefix + sessionID

	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, ErrSessionNotFound
		}

		return 0, fmt.Errorf("get session %q: %w", key, err)
	}

	userID, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse user_id from session %q: %w", key, err)
	}

	return userID, nil
}
