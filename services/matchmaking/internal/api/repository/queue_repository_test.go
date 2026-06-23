package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/pizdagladki/full/services/matchmaking/internal/api/domain"
)

func newTestRedis(t *testing.T) *redis.Client {
	t.Helper()

	mr := miniredis.RunT(t)

	return redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

func newTestRedisWithMR(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)

	return redis.NewClient(&redis.Options{Addr: mr.Addr()}), mr
}

func TestQueueRepository_EnqueueAndList(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	client := newTestRedis(t)
	repo := NewQueueRepository(client)

	p1 := domain.Player{UserID: 1, Mode: "ranked", Level: 5, EnqueuedAt: time.Now()}
	p2 := domain.Player{UserID: 2, Mode: "ranked", Level: 7, EnqueuedAt: time.Now()}

	tests := []struct {
		name        string
		enqueue     []domain.Player
		mode        string
		wantLen     int
		wantUserIDs []int64
	}{
		{
			name:        "empty queue returns empty slice",
			enqueue:     nil,
			mode:        "ranked",
			wantLen:     0,
			wantUserIDs: nil,
		},
		{
			name:        "single enqueue",
			enqueue:     []domain.Player{p1},
			mode:        "ranked",
			wantLen:     1,
			wantUserIDs: []int64{1},
		},
		{
			name:        "two enqueues same mode",
			enqueue:     []domain.Player{p1, p2},
			mode:        "ranked",
			wantLen:     2,
			wantUserIDs: []int64{1, 2},
		},
		{
			name:    "different mode returns empty",
			enqueue: []domain.Player{p1},
			mode:    "casual",
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := newTestRedis(t)
			r := NewQueueRepository(c)

			for _, p := range tt.enqueue {
				if err := r.Enqueue(ctx, p); err != nil {
					t.Fatalf("Enqueue() error = %v", err)
				}
			}

			got, err := r.ListWaiting(ctx, tt.mode)
			if err != nil {
				t.Fatalf("ListWaiting() error = %v", err)
			}
			if len(got) != tt.wantLen {
				t.Errorf("ListWaiting() len = %d, want %d", len(got), tt.wantLen)
			}

			for _, wantID := range tt.wantUserIDs {
				found := false
				for _, p := range got {
					if p.UserID == wantID {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("ListWaiting() missing userID %d", wantID)
				}
			}
		})
	}

	_ = repo // used only in parent scope to verify interface
}

func TestQueueRepository_Remove(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	p := domain.Player{UserID: 10, Mode: "ranked", Level: 5, EnqueuedAt: time.Now()}

	tests := []struct {
		name    string
		pre     bool // enqueue before remove
		wantOK  bool
		wantErr bool
	}{
		{"removes existing entry", true, true, false},
		{"removes absent entry returns false", false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := NewQueueRepository(newTestRedis(t))
			if tt.pre {
				if err := r.Enqueue(ctx, p); err != nil {
					t.Fatalf("Enqueue() error = %v", err)
				}
			}

			ok, err := r.Remove(ctx, p.Mode, p.UserID)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Remove() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if ok != tt.wantOK {
				t.Errorf("Remove() = %v, want %v", ok, tt.wantOK)
			}
		})
	}
}

func TestQueueRepository_Pair(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pa := domain.Player{UserID: 1, Mode: "ranked", Level: 5, EnqueuedAt: time.Now()}
	pb := domain.Player{UserID: 2, Mode: "ranked", Level: 6, EnqueuedAt: time.Now()}

	tests := []struct {
		name    string
		preA    bool
		preB    bool
		wantOK  bool
		wantErr bool
	}{
		{
			name:   "both present → success, both removed",
			preA:   true,
			preB:   true,
			wantOK: true,
		},
		{
			name:   "a missing → fails",
			preA:   false,
			preB:   true,
			wantOK: false,
		},
		{
			name:   "b missing → fails",
			preA:   true,
			preB:   false,
			wantOK: false,
		},
		{
			name:   "both missing → fails",
			preA:   false,
			preB:   false,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := NewQueueRepository(newTestRedis(t))
			if tt.preA {
				if err := r.Enqueue(ctx, pa); err != nil {
					t.Fatalf("Enqueue(a) error = %v", err)
				}
			}
			if tt.preB {
				if err := r.Enqueue(ctx, pb); err != nil {
					t.Fatalf("Enqueue(b) error = %v", err)
				}
			}

			ok, err := r.Pair(ctx, pa, pb)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Pair() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if ok != tt.wantOK {
				t.Errorf("Pair() = %v, want %v", ok, tt.wantOK)
			}

			if ok {
				// Verify both are actually gone.
				remaining, listErr := r.ListWaiting(ctx, pa.Mode)
				if listErr != nil {
					t.Fatalf("ListWaiting after Pair: %v", listErr)
				}
				for _, p := range remaining {
					if p.UserID == pa.UserID || p.UserID == pb.UserID {
						t.Errorf("Pair() succeeded but user %d still in queue", p.UserID)
					}
				}
			}
		})
	}
}

func TestQueueRepository_Pair_RaceCondition(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pa := domain.Player{UserID: 100, Mode: "ranked", Level: 5, EnqueuedAt: time.Now()}
	pb := domain.Player{UserID: 200, Mode: "ranked", Level: 6, EnqueuedAt: time.Now()}

	r := NewQueueRepository(newTestRedis(t))

	if err := r.Enqueue(ctx, pa); err != nil {
		t.Fatalf("Enqueue(a): %v", err)
	}
	if err := r.Enqueue(ctx, pb); err != nil {
		t.Fatalf("Enqueue(b): %v", err)
	}

	// First Pair succeeds.
	ok1, err := r.Pair(ctx, pa, pb)
	if err != nil {
		t.Fatalf("Pair#1 error = %v", err)
	}
	if !ok1 {
		t.Fatal("Pair#1 = false, want true")
	}

	// Second Pair on the same pair must fail (both already gone).
	ok2, err := r.Pair(ctx, pa, pb)
	if err != nil {
		t.Fatalf("Pair#2 error = %v", err)
	}
	if ok2 {
		t.Fatal("Pair#2 = true, want false (race guard)")
	}
}

func TestQueueRepository_EnqueueLevel(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	p := domain.Player{UserID: 42, Mode: "ranked", Level: 7, EnqueuedAt: time.Now()}

	r := NewQueueRepository(newTestRedis(t))
	if err := r.Enqueue(ctx, p); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	players, err := r.ListWaiting(ctx, "ranked")
	if err != nil {
		t.Fatalf("ListWaiting: %v", err)
	}
	if len(players) != 1 {
		t.Fatalf("len = %d, want 1", len(players))
	}
	if players[0].Level != 7 {
		t.Errorf("Level = %d, want 7", players[0].Level)
	}
}

// TestQueueRepository_Enqueue_SetsTTL asserts that Enqueue sets a backstop
// TTL on the queue hash so orphaned entries self-expire on crash.
func TestQueueRepository_Enqueue_SetsTTL(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	client, mr := newTestRedisWithMR(t)
	p := domain.Player{UserID: 99, Mode: "ranked", Level: 5, EnqueuedAt: time.Now()}

	r := NewQueueRepository(client)
	if err := r.Enqueue(ctx, p); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// miniredis exposes the TTL directly.
	ttl := mr.TTL(queueKey("ranked"))
	if ttl <= 0 {
		t.Errorf("TTL after Enqueue = %v, want > 0 (backstop TTL must be set)", ttl)
	}

	// The TTL must not exceed queueTTL.
	if ttl > queueTTL {
		t.Errorf("TTL %v > queueTTL %v", ttl, queueTTL)
	}
}

// TestQueueRepository_Enqueue_TTL_Orphan_Expires verifies the backstop: after
// the TTL elapses the hash is gone and ListWaiting returns empty.
func TestQueueRepository_Enqueue_TTL_Orphan_Expires(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	client, mr := newTestRedisWithMR(t)
	p := domain.Player{UserID: 7, Mode: "ranked", Level: 5, EnqueuedAt: time.Now()}

	r := NewQueueRepository(client)
	if err := r.Enqueue(ctx, p); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Fast-forward past the TTL.
	mr.FastForward(queueTTL + time.Second)

	players, err := r.ListWaiting(ctx, "ranked")
	if err != nil {
		t.Fatalf("ListWaiting after expiry: %v", err)
	}
	if len(players) != 0 {
		t.Errorf("expected empty queue after TTL expiry, got %d players", len(players))
	}
}
