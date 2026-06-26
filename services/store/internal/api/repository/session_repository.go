package repository

import (
	"context"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
)

const sessionKeyPrefix = "session:"

type sessionRepository struct {
	client *redis.Client
}

// NewSessionRepository returns a SessionRepository backed by a Redis client.
func NewSessionRepository(client *redis.Client) SessionRepository {
	return &sessionRepository{client: client}
}

func (r *sessionRepository) UserIDBySession(ctx context.Context, sessionID string) (int64, error) {
	key := sessionKeyPrefix + sessionID

	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil { //nolint:errorlint // redis.Nil is a sentinel value, not wrapped
			return 0, ErrSessionNotFound
		}

		return 0, fmt.Errorf("get session: %w", err)
	}

	userID, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse session user id: %w", err)
	}

	return userID, nil
}
