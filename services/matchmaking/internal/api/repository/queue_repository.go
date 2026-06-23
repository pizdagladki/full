package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/pizdagladki/full/services/matchmaking/internal/api/domain"
)

// queueTTL is a backstop TTL set on mm:queue:<mode> hashes after every
// Enqueue. If a service instance crashes without calling Remove, the hash
// self-expires after this duration so no phantom entries linger forever.
// The value is intentionally generous — it is a safety net, not a timeout.
const queueTTL = 5 * time.Minute

// queueKey returns the Redis hash key for the given mode.
// Layout: mm:queue:<mode>   field=userID (base-10)   value=JSON{level,enqueued_at}
func queueKey(mode string) string {
	return "mm:queue:" + mode
}

// queueEntry is the JSON value stored in the queue hash.
type queueEntry struct {
	Level      int       `json:"level"`
	EnqueuedAt time.Time `json:"enqueued_at"`
}

// queueRepository is the Redis-backed QueueRepository implementation.
type queueRepository struct {
	client *redis.Client
}

// NewQueueRepository returns a QueueRepository backed by the given Redis client.
func NewQueueRepository(client *redis.Client) QueueRepository {
	return &queueRepository{client: client}
}

// Enqueue adds player to the per-mode hash in Redis and refreshes the
// backstop TTL on the hash key.
func (r *queueRepository) Enqueue(ctx context.Context, player domain.Player) error {
	entry := queueEntry{
		Level:      player.Level,
		EnqueuedAt: player.EnqueuedAt,
	}

	val, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal queue entry: %w", err)
	}

	key := queueKey(player.Mode)
	field := strconv.FormatInt(player.UserID, 10)

	pipe := r.client.Pipeline()
	pipe.HSet(ctx, key, field, val)
	pipe.Expire(ctx, key, queueTTL)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("enqueue player: %w", err)
	}

	return nil
}

// ListWaiting returns all players waiting in the given mode.
func (r *queueRepository) ListWaiting(ctx context.Context, mode string) ([]domain.Player, error) {
	raw, err := r.client.HGetAll(ctx, queueKey(mode)).Result()
	if err != nil {
		return nil, fmt.Errorf("hgetall queue %q: %w", mode, err)
	}

	players := make([]domain.Player, 0, len(raw))

	for field, val := range raw {
		userID, parseErr := strconv.ParseInt(field, 10, 64)
		if parseErr != nil {
			continue // skip corrupted entries
		}

		var entry queueEntry
		unmarshalErr := json.Unmarshal([]byte(val), &entry)
		if unmarshalErr != nil {
			continue
		}

		players = append(players, domain.Player{
			UserID:     userID,
			Mode:       mode,
			Level:      entry.Level,
			EnqueuedAt: entry.EnqueuedAt,
		})
	}

	return players, nil
}

// Remove deletes the player's field from the queue hash. Returns true iff the
// field was present (i.e. this call actually removed it).
func (r *queueRepository) Remove(ctx context.Context, mode string, userID int64) (bool, error) {
	field := strconv.FormatInt(userID, 10)

	n, err := r.client.HDel(ctx, queueKey(mode), field).Result()
	if err != nil {
		return false, fmt.Errorf("hdel queue %q user %d: %w", mode, userID, err)
	}

	return n > 0, nil
}

// pairScript atomically removes both players from the queue only when BOTH are
// still present. KEYS[1] and KEYS[2] are the queue hash keys for a and b.
// ARGV[1] and ARGV[2] are the field names (userIDs as strings). Returns 1 on
// success, 0 if either field is missing.
var pairScript = redis.NewScript(`
local ka  = KEYS[1]
local kb  = KEYS[2]
local fa  = ARGV[1]
local fb  = ARGV[2]
if redis.call("HEXISTS", ka, fa) == 0 then return 0 end
if redis.call("HEXISTS", kb, fb) == 0 then return 0 end
redis.call("HDEL", ka, fa)
redis.call("HDEL", kb, fb)
return 1
`)

// Pair atomically removes both players from the queue. Returns false when
// either player is already gone (lost the race — no match should be emitted).
func (r *queueRepository) Pair(ctx context.Context, a, b domain.Player) (bool, error) {
	ka := queueKey(a.Mode)
	kb := queueKey(b.Mode)
	fa := strconv.FormatInt(a.UserID, 10)
	fb := strconv.FormatInt(b.UserID, 10)

	result, err := pairScript.Run(ctx, r.client, []string{ka, kb}, fa, fb).Int()
	if err != nil {
		return false, fmt.Errorf("pair script: %w", err)
	}

	return result == 1, nil
}
