package repository

import (
	"context"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
)

const pointsBalanceKeyPrefix = "points:balance:"

type pointsCacheRepository struct {
	client *redis.Client
}

// NewPointsCache returns a PointsCache backed by a Redis client.
func NewPointsCache(client *redis.Client) PointsCache {
	return &pointsCacheRepository{client: client}
}

func pointsBalanceKey(userID int64) string {
	return pointsBalanceKeyPrefix + strconv.FormatInt(userID, 10)
}

func (r *pointsCacheRepository) GetBalance(ctx context.Context, userID int64) (int64, bool, error) {
	val, err := r.client.Get(ctx, pointsBalanceKey(userID)).Result()
	if err != nil {
		if err == redis.Nil { //nolint:errorlint // redis.Nil is a sentinel value, not wrapped
			return 0, false, nil
		}

		return 0, false, fmt.Errorf("get cached points balance: %w", err)
	}

	balance, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("parse cached points balance: %w", err)
	}

	return balance, true, nil
}

func (r *pointsCacheRepository) SetBalance(ctx context.Context, userID, balance int64) error {
	err := r.client.Set(ctx, pointsBalanceKey(userID), balance, 0).Err()
	if err != nil {
		return fmt.Errorf("set cached points balance: %w", err)
	}

	return nil
}

func (r *pointsCacheRepository) DeleteBalance(ctx context.Context, userID int64) error {
	err := r.client.Del(ctx, pointsBalanceKey(userID)).Err()
	if err != nil {
		return fmt.Errorf("delete cached points balance: %w", err)
	}

	return nil
}
