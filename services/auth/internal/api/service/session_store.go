package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const sessionKeyPrefix = "session:"

type redisSessionStore struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRedisSessionStore returns a SessionStore backed by Redis with the given TTL.
func NewRedisSessionStore(client *redis.Client, ttl time.Duration) SessionStore {
	return &redisSessionStore{client: client, ttl: ttl}
}

func generateSessionID() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *redisSessionStore) Create(ctx context.Context, userID int64) (string, error) {
	id, err := generateSessionID()
	if err != nil {
		return "", err
	}

	key := sessionKeyPrefix + id
	val := strconv.FormatInt(userID, 10)

	err = s.client.Set(ctx, key, val, s.ttl).Err()
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}

	return id, nil
}

func (s *redisSessionStore) Get(ctx context.Context, sessionID string) (int64, error) {
	key := sessionKeyPrefix + sessionID

	val, err := s.client.Get(ctx, key).Result()
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

func (s *redisSessionStore) Delete(ctx context.Context, sessionID string) error {
	key := sessionKeyPrefix + sessionID

	err := s.client.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return nil
}
