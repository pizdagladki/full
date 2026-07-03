package repository

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// codeAlphabet excludes visually-ambiguous characters (0/O, 1/I) so a shared
// code is easy to read and type back.
const codeAlphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"

const (
	// codeLen is the length of a generated invite code.
	codeLen = 7
	// maxCreateAttempts bounds the retries on a Redis SET NX collision.
	maxCreateAttempts = 5
)

// roomCodeKey returns the Redis STRING key for a code→room mapping.
// Layout: roomcode:<code>   value=roomID
func roomCodeKey(code string) string {
	return "roomcode:" + code
}

// roomCodeRepository is the Redis-backed RoomCodeRepository implementation.
type roomCodeRepository struct {
	client  *redis.Client
	codeTTL time.Duration
}

// NewRoomCodeRepository returns a RoomCodeRepository backed by the given Redis
// client. codeTTL is applied to every created code.
func NewRoomCodeRepository(client *redis.Client, codeTTL time.Duration) RoomCodeRepository {
	return &roomCodeRepository{client: client, codeTTL: codeTTL}
}

// CreateCode generates a short unique invite code and stores code→roomID with
// the configured TTL, retrying a bounded number of times on collision.
func (r *roomCodeRepository) CreateCode(ctx context.Context, roomID string) (string, error) {
	for range maxCreateAttempts {
		code, err := generateCode()
		if err != nil {
			return "", fmt.Errorf("generate invite code: %w", err)
		}

		ok, setErr := r.client.SetNX(ctx, roomCodeKey(code), roomID, r.codeTTL).Result()
		if setErr != nil {
			return "", fmt.Errorf("set invite code %q for room %q: %w", code, roomID, setErr)
		}

		if ok {
			return code, nil
		}
		// Collision — another room already holds this code. Retry.
	}

	return "", fmt.Errorf("create invite code for room %q: exhausted %d attempts", roomID, maxCreateAttempts)
}

// ResolveCode returns the roomID mapped to code, or ErrCodeNotFound if the
// code is absent or expired.
func (r *roomCodeRepository) ResolveCode(ctx context.Context, code string) (string, error) {
	roomID, err := r.client.Get(ctx, roomCodeKey(code)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", ErrCodeNotFound
		}

		return "", fmt.Errorf("get invite code %q: %w", code, err)
	}

	return roomID, nil
}

// RemoveCode deletes the code→room mapping.
func (r *roomCodeRepository) RemoveCode(ctx context.Context, code string) error {
	err := r.client.Del(ctx, roomCodeKey(code)).Err()
	if err != nil {
		return fmt.Errorf("del invite code %q: %w", code, err)
	}

	return nil
}

// generateCode returns a random codeLen-character string drawn from
// codeAlphabet using crypto/rand.
func generateCode() (string, error) {
	buf := make([]byte, codeLen)

	_, err := rand.Read(buf)
	if err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}

	out := make([]byte, codeLen)
	alphabetLen := len(codeAlphabet)

	for i, v := range buf {
		out[i] = codeAlphabet[int(v)%alphabetLen]
	}

	return string(out), nil
}
