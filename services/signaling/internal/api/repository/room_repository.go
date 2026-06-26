package repository

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// roomKey returns the Redis SET key for a room's member list.
// Layout: room:<roomID>:members   values=userIDs (base-10 strings)
func roomKey(roomID string) string {
	return "room:" + roomID + ":members"
}

// joinScript atomically tries to add a member to the room set.
// KEYS[1] = room:<roomID>:members
// ARGV[1] = userID (base-10 string)
// ARGV[2] = room TTL in seconds (string)
//
// Return values:
//
//	0 → joined (SADD + EXPIRE)
//	1 → already a member (SISMEMBER was true)
//	2 → full (SCARD ≥ 2 and caller not already a member)
var joinScript = redis.NewScript(`
local key    = KEYS[1]
local uid    = ARGV[1]
local ttl    = tonumber(ARGV[2])

-- Idempotent: if the caller is already in the set, refresh TTL and report that.
if redis.call("SISMEMBER", key, uid) == 1 then
  redis.call("EXPIRE", key, ttl)
  return 1
end
-- Reject a third joiner (no TTL refresh — full room, caller not admitted).
if redis.call("SCARD", key) >= 2 then
  return 2
end
-- Add and (re-)set the TTL.
redis.call("SADD", key, uid)
redis.call("EXPIRE", key, ttl)
return 0
`)

// roomRepository is the Redis-backed RoomRepository implementation.
type roomRepository struct {
	client  *redis.Client
	roomTTL time.Duration
}

// NewRoomRepository returns a RoomRepository backed by the given Redis client.
// roomTTL is applied to room keys on every successful Join.
func NewRoomRepository(client *redis.Client, roomTTL time.Duration) RoomRepository {
	return &roomRepository{client: client, roomTTL: roomTTL}
}

// Join atomically adds userID to the room set.
func (r *roomRepository) Join(ctx context.Context, roomID string, userID int64) (JoinResult, error) {
	key := roomKey(roomID)
	uid := strconv.FormatInt(userID, 10)
	ttlSecs := int64(r.roomTTL.Seconds())

	result, err := joinScript.Run(ctx, r.client, []string{key}, uid, ttlSecs).Int()
	if err != nil {
		return 0, fmt.Errorf("join script room %q user %d: %w", roomID, userID, err)
	}

	switch result {
	case 0:
		return JoinResultJoined, nil
	case 1:
		return JoinResultAlreadyMember, nil
	default:
		return JoinResultFull, nil
	}
}

// IsMember reports whether userID is a member of roomID.
func (r *roomRepository) IsMember(ctx context.Context, roomID string, userID int64) (bool, error) {
	uid := strconv.FormatInt(userID, 10)

	ok, err := r.client.SIsMember(ctx, roomKey(roomID), uid).Result()
	if err != nil {
		return false, fmt.Errorf("sismember room %q user %d: %w", roomID, userID, err)
	}

	return ok, nil
}

// RemoveRoom deletes the room membership key from Redis.
func (r *roomRepository) RemoveRoom(ctx context.Context, roomID string) error {
	err := r.client.Del(ctx, roomKey(roomID)).Err()
	if err != nil {
		return fmt.Errorf("del room %q: %w", roomID, err)
	}

	return nil
}
