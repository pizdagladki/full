package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/pizdagladki/full/services/auth/internal/api/service"
)

func newTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	return client, mr
}

func TestRedisSessionStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(t *testing.T, store service.SessionStore, mr *miniredis.Miniredis)
	}{
		{
			name: "Create and Get round-trip",
			run: func(t *testing.T, store service.SessionStore, _ *miniredis.Miniredis) {
				t.Helper()

				ctx := context.Background()

				sessionID, err := store.Create(ctx, 42)
				if err != nil {
					t.Fatalf("Create() error = %v", err)
				}
				if sessionID == "" {
					t.Fatal("Create() returned empty sessionID")
				}

				userID, err := store.Get(ctx, sessionID)
				if err != nil {
					t.Fatalf("Get() error = %v", err)
				}
				if userID != 42 {
					t.Errorf("Get() userID = %d, want 42", userID)
				}
			},
		},
		{
			name: "Get missing session returns ErrSessionNotFound",
			run: func(t *testing.T, store service.SessionStore, _ *miniredis.Miniredis) {
				t.Helper()

				_, err := store.Get(context.Background(), "no-such-session")
				if !errors.Is(err, service.ErrSessionNotFound) {
					t.Errorf("Get() error = %v, want ErrSessionNotFound", err)
				}
			},
		},
		{
			name: "Delete removes the session",
			run: func(t *testing.T, store service.SessionStore, _ *miniredis.Miniredis) {
				t.Helper()

				ctx := context.Background()

				sessionID, err := store.Create(ctx, 7)
				if err != nil {
					t.Fatalf("Create() error = %v", err)
				}

				if err = store.Delete(ctx, sessionID); err != nil {
					t.Fatalf("Delete() error = %v", err)
				}

				_, err = store.Get(ctx, sessionID)
				if !errors.Is(err, service.ErrSessionNotFound) {
					t.Errorf("Get() after Delete error = %v, want ErrSessionNotFound", err)
				}
			},
		},
		{
			name: "expired session returns ErrSessionNotFound",
			run: func(t *testing.T, store service.SessionStore, mr *miniredis.Miniredis) {
				t.Helper()

				ctx := context.Background()

				sessionID, err := store.Create(ctx, 5)
				if err != nil {
					t.Fatalf("Create() error = %v", err)
				}

				// Fast-forward miniredis time past the TTL (1 second used in test).
				mr.FastForward(2 * time.Second)

				_, err = store.Get(ctx, sessionID)
				if !errors.Is(err, service.ErrSessionNotFound) {
					t.Errorf("Get() after expiry error = %v, want ErrSessionNotFound", err)
				}
			},
		},
		{
			name: "Create produces unique session IDs",
			run: func(t *testing.T, store service.SessionStore, _ *miniredis.Miniredis) {
				t.Helper()

				ctx := context.Background()

				id1, err := store.Create(ctx, 1)
				if err != nil {
					t.Fatalf("Create() first error = %v", err)
				}

				id2, err := store.Create(ctx, 1)
				if err != nil {
					t.Fatalf("Create() second error = %v", err)
				}

				if id1 == id2 {
					t.Error("Create() produced identical session IDs")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, mr := newTestRedis(t)
			// Use a 1-second TTL so expiry tests don't take long.
			store := service.NewRedisSessionStore(client, time.Second)

			tt.run(t, store, mr)
		})
	}
}
