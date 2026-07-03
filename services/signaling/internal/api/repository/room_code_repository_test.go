package repository

import (
	"context"
	"errors"
	"testing"
	"time"
)

const testCodeTTL = 15 * time.Minute

func TestRoomCodeRepository_CreateCode(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name   string
		roomID string
	}{
		{name: "create returns a non-empty code and stores the mapping", roomID: "room-create-1"},  // criterion: 1
		{name: "create for a different room returns a different mapping", roomID: "room-create-2"}, // criterion: 1
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, _ := newTestRedis(t)
			repo := NewRoomCodeRepository(client, testCodeTTL)

			code, err := repo.CreateCode(ctx, tt.roomID)
			if err != nil {
				t.Fatalf("CreateCode() error = %v", err)
			}

			if code == "" {
				t.Fatal("CreateCode() = \"\", want a non-empty code") // criterion: 1 — fails if no code returned
			}

			gotRoomID, resolveErr := repo.ResolveCode(ctx, code)
			if resolveErr != nil {
				t.Fatalf("ResolveCode(%q) error = %v", code, resolveErr)
			}

			if gotRoomID != tt.roomID {
				t.Errorf("ResolveCode(%q) = %q, want %q", code, gotRoomID, tt.roomID) // criterion: 1 — fails if mapping not stored
			}
		})
	}
}

func TestRoomCodeRepository_ResolveCode(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("resolve returns the room id for a valid code", func(t *testing.T) {
		t.Parallel()

		client, _ := newTestRedis(t)
		repo := NewRoomCodeRepository(client, testCodeTTL)

		code, err := repo.CreateCode(ctx, "room-resolve")
		if err != nil {
			t.Fatalf("CreateCode() error = %v", err)
		}

		roomID, err := repo.ResolveCode(ctx, code)
		if err != nil {
			t.Fatalf("ResolveCode() error = %v", err)
		}

		if roomID != "room-resolve" {
			t.Errorf("ResolveCode() = %q, want %q", roomID, "room-resolve") // criterion: 2
		}
	})

	t.Run("resolve of an unknown code returns ErrCodeNotFound", func(t *testing.T) {
		t.Parallel()

		client, _ := newTestRedis(t)
		repo := NewRoomCodeRepository(client, testCodeTTL)

		_, err := repo.ResolveCode(ctx, "NOSUCH1")
		if !errors.Is(err, ErrCodeNotFound) {
			t.Errorf("ResolveCode() error = %v, want ErrCodeNotFound", err) // criterion: 2 — fails if unknown code not rejected
		}
	})

	t.Run("resolve of an expired code returns ErrCodeNotFound", func(t *testing.T) {
		t.Parallel()

		client, mr := newTestRedis(t)
		shortTTL := 50 * time.Millisecond
		repo := NewRoomCodeRepository(client, shortTTL)

		code, err := repo.CreateCode(ctx, "room-expiring")
		if err != nil {
			t.Fatalf("CreateCode() error = %v", err)
		}

		mr.FastForward(shortTTL + time.Second)

		_, err = repo.ResolveCode(ctx, code)
		if !errors.Is(err, ErrCodeNotFound) {
			t.Errorf("ResolveCode() after expiry error = %v, want ErrCodeNotFound", err) // criterion: 2 — fails if expiry not enforced
		}
	})
}

func TestRoomCodeRepository_RemoveCode(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("remove deletes the code mapping", func(t *testing.T) {
		t.Parallel()

		client, _ := newTestRedis(t)
		repo := NewRoomCodeRepository(client, testCodeTTL)

		code, err := repo.CreateCode(ctx, "room-remove")
		if err != nil {
			t.Fatalf("CreateCode() error = %v", err)
		}

		if err := repo.RemoveCode(ctx, code); err != nil {
			t.Fatalf("RemoveCode() error = %v", err)
		}

		_, resolveErr := repo.ResolveCode(ctx, code)
		if !errors.Is(resolveErr, ErrCodeNotFound) {
			t.Errorf("ResolveCode() after RemoveCode error = %v, want ErrCodeNotFound", resolveErr) // criterion: 3 — fails if code not removed
		}
	})

	t.Run("remove of a non-existent code is a no-op", func(t *testing.T) {
		t.Parallel()

		client, _ := newTestRedis(t)
		repo := NewRoomCodeRepository(client, testCodeTTL)

		if err := repo.RemoveCode(ctx, "GHOST01"); err != nil {
			t.Fatalf("RemoveCode() error = %v, want nil for non-existent code", err)
		}
	})
}

func TestRoomCodeRepository_CreateCode_SetsTTL(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	client, mr := newTestRedis(t)
	repo := NewRoomCodeRepository(client, testCodeTTL)

	code, err := repo.CreateCode(ctx, "room-ttl")
	if err != nil {
		t.Fatalf("CreateCode() error = %v", err)
	}

	ttl := mr.TTL(roomCodeKey(code))
	if ttl <= 0 {
		t.Errorf("TTL after CreateCode = %v, want > 0", ttl)
	}

	if ttl > testCodeTTL {
		t.Errorf("TTL %v > testCodeTTL %v", ttl, testCodeTTL)
	}
}
