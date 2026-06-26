package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/pizdagladki/full/services/reports/internal/api/repository"
)

func newTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	return client, mr
}

func TestCooldownStore(t *testing.T) {
	t.Parallel()

	const userID = int64(42)
	const ttl = 30 * time.Minute

	tests := []struct {
		name string
		run  func(t *testing.T, store repository.CooldownStore, mr *miniredis.Miniredis)
	}{
		{
			// criterion: 2 — SetCooldown writes key with correct TTL
			name: "SetCooldown writes key that is then active",
			run: func(t *testing.T, store repository.CooldownStore, _ *miniredis.Miniredis) {
				t.Helper()

				if err := store.SetCooldown(context.Background(), userID, ttl); err != nil {
					t.Fatalf("SetCooldown() error = %v", err)
				}

				active, secs, err := store.GetCooldown(context.Background(), userID)
				if err != nil {
					t.Fatalf("GetCooldown() error = %v", err)
				}

				if !active {
					t.Error("active = false, want true after SetCooldown")
				}

				wantSecs := int(ttl.Seconds())
				// Allow 1 second of tolerance for timing.
				if secs < wantSecs-1 || secs > wantSecs {
					t.Errorf("seconds_remaining = %d, want ~%d", secs, wantSecs)
				}
			},
		},
		{
			// criterion: 3 — GetCooldown inactive when key absent
			name: "GetCooldown returns inactive when no key",
			run: func(t *testing.T, store repository.CooldownStore, _ *miniredis.Miniredis) {
				t.Helper()

				active, secs, err := store.GetCooldown(context.Background(), userID+1)
				if err != nil {
					t.Fatalf("GetCooldown() error = %v", err)
				}

				if active {
					t.Error("active = true, want false for missing key")
				}

				if secs != 0 {
					t.Errorf("seconds_remaining = %d, want 0 for missing key", secs)
				}
			},
		},
		{
			// criterion: 3 — GetCooldown returns inactive after key expires
			name: "GetCooldown inactive after key expires",
			run: func(t *testing.T, store repository.CooldownStore, mr *miniredis.Miniredis) {
				t.Helper()

				if err := store.SetCooldown(context.Background(), userID, time.Second); err != nil {
					t.Fatalf("SetCooldown() error = %v", err)
				}

				// Fast-forward miniredis time past the TTL.
				mr.FastForward(2 * time.Second)

				active, secs, err := store.GetCooldown(context.Background(), userID)
				if err != nil {
					t.Fatalf("GetCooldown() after expiry error = %v", err)
				}

				if active {
					t.Error("active = true after expiry, want false")
				}

				if secs != 0 {
					t.Errorf("seconds_remaining = %d after expiry, want 0", secs)
				}
			},
		},
		{
			// criterion: 2/3 — SetCooldown twice overwrites TTL (idempotent)
			name: "SetCooldown twice overwrites previous TTL",
			run: func(t *testing.T, store repository.CooldownStore, _ *miniredis.Miniredis) {
				t.Helper()

				if err := store.SetCooldown(context.Background(), userID, ttl); err != nil {
					t.Fatalf("SetCooldown() first error = %v", err)
				}

				newTTL := 10 * time.Minute
				if err := store.SetCooldown(context.Background(), userID, newTTL); err != nil {
					t.Fatalf("SetCooldown() second error = %v", err)
				}

				active, secs, err := store.GetCooldown(context.Background(), userID)
				if err != nil {
					t.Fatalf("GetCooldown() error = %v", err)
				}

				if !active {
					t.Error("active = false, want true after second SetCooldown")
				}

				wantSecs := int(newTTL.Seconds())
				if secs < wantSecs-1 || secs > wantSecs {
					t.Errorf("seconds_remaining = %d, want ~%d", secs, wantSecs)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, mr := newTestRedis(t)
			store := repository.NewCooldownStore(client)

			tt.run(t, store, mr)
		})
	}
}
