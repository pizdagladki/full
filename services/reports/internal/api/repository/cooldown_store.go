package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const cooldownKeyPrefix = "report:cooldown:"

type redisCooldownStore struct {
	client *redis.Client
}

// NewCooldownStore constructs a Redis-backed CooldownStore.
func NewCooldownStore(client *redis.Client) CooldownStore {
	return &redisCooldownStore{client: client}
}

// SetCooldown writes a cooldown key for userID with the given TTL.
func (s *redisCooldownStore) SetCooldown(ctx context.Context, userID int64, ttl time.Duration) error {
	key := fmt.Sprintf("%s%d", cooldownKeyPrefix, userID)

	err := s.client.Set(ctx, key, 1, ttl).Err()
	if err != nil {
		return fmt.Errorf("set cooldown for user %d: %w", userID, err)
	}

	return nil
}

// GetCooldown returns whether a cooldown is active and how many seconds remain.
// Returns active=false when the key is absent (-2) or has no expiry (-1).
func (s *redisCooldownStore) GetCooldown(ctx context.Context, userID int64) (bool, int, error) {
	key := fmt.Sprintf("%s%d", cooldownKeyPrefix, userID)

	ttl, err := s.client.TTL(ctx, key).Result()
	if err != nil {
		return false, 0, fmt.Errorf("get cooldown ttl for user %d: %w", userID, err)
	}

	// -2 means key does not exist; -1 means key exists without expiry.
	if ttl <= 0 {
		return false, 0, nil
	}

	return true, int(ttl.Seconds()), nil
}
