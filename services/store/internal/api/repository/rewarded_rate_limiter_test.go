package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/pizdagladki/full/services/store/internal/api/repository"
)

func TestRewardedRateLimiter_Allow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		cap        int
		window     time.Duration
		userID     int64
		attempts   int // number of consecutive Allow() calls
		wantAllows []bool
	}{
		{
			// criterion: 3 — attempts up to the cap are allowed.
			name:       "allows up to cap",
			cap:        3,
			window:     time.Hour,
			userID:     1,
			attempts:   3,
			wantAllows: []bool{true, true, true},
		},
		{
			// criterion: 3 — the (cap+1)-th attempt within the window is denied
			// (rate-limited), and no further grant happens.
			name:       "denies over cap",
			cap:        2,
			window:     time.Hour,
			userID:     2,
			attempts:   3,
			wantAllows: []bool{true, true, false},
		},
		{
			// criterion: 3 — a non-positive cap denies every attempt.
			name:       "non-positive cap denies always",
			cap:        0,
			window:     time.Hour,
			userID:     3,
			attempts:   1,
			wantAllows: []bool{false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mr := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			defer client.Close()

			limiter := repository.NewRewardedRateLimiter(client, tt.cap, tt.window)

			for i := 0; i < tt.attempts; i++ {
				allowed, err := limiter.Allow(context.Background(), tt.userID)
				if err != nil {
					t.Fatalf("Allow() attempt %d unexpected error = %v", i, err)
				}

				if allowed != tt.wantAllows[i] {
					t.Errorf("Allow() attempt %d = %v, want %v", i, allowed, tt.wantAllows[i])
				}
			}
		})
	}
}

func TestRewardedRateLimiter_Allow_SetsWindowExpiry(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	limiter := repository.NewRewardedRateLimiter(client, 5, 30*time.Minute)

	allowed, err := limiter.Allow(context.Background(), 42)
	if err != nil {
		t.Fatalf("Allow() unexpected error = %v", err)
	}

	if !allowed {
		t.Fatal("Allow() = false, want true on first attempt")
	}

	// criterion: 3 — the fixed window key must carry a bounded TTL matching the
	// configured window, set on the first attempt of the window.
	ttl := mr.TTL("rewarded:grant:42")
	if ttl <= 0 {
		t.Errorf("TTL on rewarded:grant:42 = %v, want a positive bounded TTL", ttl)
	}

	if ttl > 30*time.Minute {
		t.Errorf("TTL on rewarded:grant:42 = %v, want <= 30m", ttl)
	}
}

func TestRewardedRateLimiter_Allow_RedisErrorPropagates(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	_ = client.Close()

	limiter := repository.NewRewardedRateLimiter(client, 5, time.Hour)

	_, err := limiter.Allow(context.Background(), 1)
	if err == nil {
		t.Fatal("Allow() error = nil, want error against a closed client")
	}
}
