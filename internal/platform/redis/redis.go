// Package redis provides the shared go-redis client used by services that need
// Redis for sessions, queues, caches, or cooldowns. It arrives with the first
// service that needs it and must not be duplicated inside a service.
package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// New builds a go-redis client for addr and verifies it with a ping. The client
// is closed automatically when the ping fails, so the caller never receives a
// live client together with an error.
func New(ctx context.Context, addr, password string) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
	})

	err := client.Ping(ctx).Err()
	if err != nil {
		_ = client.Close()

		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return client, nil
}
