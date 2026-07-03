package repository

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const rewardedGrantKeyPrefix = "rewarded:grant:"

type rewardedRateLimiter struct {
	client *redis.Client
	cap    int
	window time.Duration
}

// NewRewardedRateLimiter returns a RewardedRateLimiter backed by a Redis
// client, allowing at most cap grants per user within a fixed window.
func NewRewardedRateLimiter(client *redis.Client, cap int, window time.Duration) RewardedRateLimiter {
	return &rewardedRateLimiter{client: client, cap: cap, window: window}
}

func rewardedGrantKey(userID int64) string {
	return rewardedGrantKeyPrefix + strconv.FormatInt(userID, 10)
}

// Allow increments the fixed-window counter for userID, setting the window
// expiry on the first attempt in the window, then reports whether the
// resulting count is within the configured cap. A non-positive cap always
// denies (there is nothing to allow); the config loader guarantees a
// positive default, but this keeps the limiter safe against misconfiguration.
func (l *rewardedRateLimiter) Allow(ctx context.Context, userID int64) (bool, error) {
	if l.cap <= 0 {
		return false, nil
	}

	key := rewardedGrantKey(userID)

	n, err := l.client.Incr(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("increment rewarded grant counter: %w", err)
	}

	if n == 1 {
		err = l.client.Expire(ctx, key, l.window).Err()
		if err != nil {
			return false, fmt.Errorf("set rewarded grant window expiry: %w", err)
		}
	}

	return n <= int64(l.cap), nil
}
