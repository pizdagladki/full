package repository

import (
	"context"
	"testing"
	"time"
)

const testRoomTTL = 30 * time.Minute

func TestRoomRepository_Join(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name       string
		setup      func(client interface{}, roomID string) // pre-populate using direct SADD
		roomID     string
		userID     int64
		wantResult JoinResult
		wantErr    bool
	}{
		{
			name:       "first joiner gets JoinResultJoined", // criterion: 1 — fails if first join not admitted
			setup:      func(_ interface{}, _ string) {},
			roomID:     "room1",
			userID:     10,
			wantResult: JoinResultJoined,
		},
		{
			name:   "second joiner gets JoinResultJoined", // criterion: 1 — fails if second join not admitted
			roomID: "room2",
			userID: 20,
			setup: func(iface interface{}, roomID string) {
				// use the repo itself to join user 10 first
			},
			wantResult: JoinResultJoined,
		},
		{
			name:       "already-member gets JoinResultAlreadyMember", // criterion: 1 — idempotent join
			setup:      func(_ interface{}, _ string) {},
			roomID:     "room3",
			userID:     30,
			wantResult: JoinResultAlreadyMember, // see sub-test below
		},
		{
			name:       "third joiner gets JoinResultFull", // criterion: 1, 3rd peer rejection
			setup:      func(_ interface{}, _ string) {},
			roomID:     "room4",
			userID:     40,
			wantResult: JoinResultFull,
		},
	}

	_ = tests // we test via dedicated subtests below to manage state properly

	t.Run("first joiner admitted", func(t *testing.T) {
		t.Parallel()

		client, _ := newTestRedis(t)
		repo := NewRoomRepository(client, testRoomTTL)

		result, err := repo.Join(ctx, "room-a", 1)
		if err != nil {
			t.Fatalf("Join() error = %v", err)
		}
		if result != JoinResultJoined {
			t.Errorf("Join() = %v, want JoinResultJoined", result)
		}
	})

	t.Run("second joiner admitted", func(t *testing.T) {
		t.Parallel()

		client, _ := newTestRedis(t)
		repo := NewRoomRepository(client, testRoomTTL)

		if _, err := repo.Join(ctx, "room-b", 1); err != nil {
			t.Fatalf("first Join() error = %v", err)
		}

		result, err := repo.Join(ctx, "room-b", 2)
		if err != nil {
			t.Fatalf("second Join() error = %v", err)
		}

		if result != JoinResultJoined {
			t.Errorf("Join() = %v, want JoinResultJoined for second joiner", result)
		}
	})

	t.Run("idempotent join returns AlreadyMember", func(t *testing.T) {
		t.Parallel()

		client, _ := newTestRedis(t)
		repo := NewRoomRepository(client, testRoomTTL)

		if _, err := repo.Join(ctx, "room-c", 1); err != nil {
			t.Fatalf("first Join() error = %v", err)
		}

		// Same user joins again — must be idempotent.
		result, err := repo.Join(ctx, "room-c", 1)
		if err != nil {
			t.Fatalf("idempotent Join() error = %v", err)
		}

		if result != JoinResultAlreadyMember {
			t.Errorf("Join() = %v, want JoinResultAlreadyMember", result) // criterion: 1 — fails if idempotent not handled
		}
	})

	t.Run("third joiner rejected with JoinResultFull", func(t *testing.T) {
		t.Parallel()

		client, _ := newTestRedis(t)
		repo := NewRoomRepository(client, testRoomTTL)

		if _, err := repo.Join(ctx, "room-d", 1); err != nil {
			t.Fatalf("Join(1) error = %v", err)
		}

		if _, err := repo.Join(ctx, "room-d", 2); err != nil {
			t.Fatalf("Join(2) error = %v", err)
		}

		result, err := repo.Join(ctx, "room-d", 3)
		if err != nil {
			t.Fatalf("Join(3) error = %v", err)
		}

		if result != JoinResultFull {
			t.Errorf("Join() = %v, want JoinResultFull for third joiner", result) // criterion: 1 — fails if third peer not rejected
		}
	})
}

func TestRoomRepository_IsMember(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name   string
		setup  func(repo RoomRepository)
		roomID string
		userID int64
		want   bool
	}{
		{
			name: "member is recognized", // criterion: 4 — IsMember is used to check relay authorization
			setup: func(repo RoomRepository) {
				_, _ = repo.Join(ctx, "room-e", 10)
			},
			roomID: "room-e",
			userID: 10,
			want:   true,
		},
		{
			name:   "non-member returns false", // criterion: 4 — fails if non-member not rejected
			setup:  func(_ RoomRepository) {},
			roomID: "room-f",
			userID: 99,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, _ := newTestRedis(t)
			repo := NewRoomRepository(client, testRoomTTL)

			tt.setup(repo)

			got, err := repo.IsMember(ctx, tt.roomID, tt.userID)
			if err != nil {
				t.Fatalf("IsMember() error = %v", err)
			}

			if got != tt.want {
				t.Errorf("IsMember() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRoomRepository_RemoveRoom(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("remove existing room cleans up members", func(t *testing.T) {
		t.Parallel()

		client, _ := newTestRedis(t)
		repo := NewRoomRepository(client, testRoomTTL)

		if _, err := repo.Join(ctx, "room-g", 1); err != nil {
			t.Fatalf("Join() error = %v", err)
		}

		if err := repo.RemoveRoom(ctx, "room-g"); err != nil {
			t.Fatalf("RemoveRoom() error = %v", err)
		}

		// After removal the member should not be found.
		ok, err := repo.IsMember(ctx, "room-g", 1)
		if err != nil {
			t.Fatalf("IsMember() error = %v", err)
		}

		if ok {
			t.Error("IsMember() = true after RemoveRoom, want false") // criterion: 5 — fails if room not cleaned up
		}
	})

	t.Run("remove non-existent room is a no-op", func(t *testing.T) {
		t.Parallel()

		client, _ := newTestRedis(t)
		repo := NewRoomRepository(client, testRoomTTL)

		if err := repo.RemoveRoom(ctx, "no-such-room"); err != nil {
			t.Fatalf("RemoveRoom() error = %v, want nil for non-existent room", err)
		}
	})
}

func TestRoomRepository_Join_SetsTTL(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	client, mr := newTestRedis(t)
	repo := NewRoomRepository(client, testRoomTTL)

	if _, err := repo.Join(ctx, "room-ttl", 1); err != nil {
		t.Fatalf("Join() error = %v", err)
	}

	ttl := mr.TTL(roomKey("room-ttl"))
	if ttl <= 0 {
		t.Errorf("TTL after Join = %v, want > 0", ttl)
	}

	if ttl > testRoomTTL {
		t.Errorf("TTL %v > testRoomTTL %v", ttl, testRoomTTL)
	}
}

func TestRoomRepository_Join_RefreshesTTL_OnReJoin(t *testing.T) {
	t.Parallel()

	// criterion: fix3 — idempotent re-join must refresh the TTL so a long-running
	// session does not expire mid-battle.
	// We verify the TTL is refreshed by reading it directly from miniredis before
	// and after the re-join with a smaller re-join TTL, then confirming the TTL
	// increased.  Fails if the already-member branch does NOT call EXPIRE.
	ctx := context.Background()

	// Use two different TTL values: first join with a short TTL, then re-join
	// with a long TTL (by swapping the repo). This proves the EXPIRE is called
	// on the already-member path without relying on time.Sleep or FastForward.
	client, mr := newTestRedis(t)

	shortTTL := 1 * time.Second
	longTTL := 10 * time.Minute

	// First join: TTL = shortTTL (1s)
	repoShort := NewRoomRepository(client, shortTTL)

	if _, err := repoShort.Join(ctx, "room-rjoin", 1); err != nil {
		t.Fatalf("initial Join() error = %v", err)
	}

	ttlAfterFirstJoin := mr.TTL(roomKey("room-rjoin"))
	if ttlAfterFirstJoin <= 0 || ttlAfterFirstJoin > shortTTL {
		t.Fatalf("TTL after first join = %v, want (0, %v]", ttlAfterFirstJoin, shortTTL)
	}

	// Re-join same user with a repo configured for longTTL — if EXPIRE is called
	// on the already-member branch the TTL will jump to longTTL.
	repoLong := NewRoomRepository(client, longTTL)

	result, err := repoLong.Join(ctx, "room-rjoin", 1)
	if err != nil {
		t.Fatalf("re-join Join() error = %v", err)
	}

	if result != JoinResultAlreadyMember {
		t.Errorf("re-join result = %v, want JoinResultAlreadyMember", result)
	}

	ttlAfterReJoin := mr.TTL(roomKey("room-rjoin"))
	// TTL must have been refreshed to longTTL (will be slightly less than longTTL
	// due to Lua execution time, but will be >> shortTTL).
	if ttlAfterReJoin <= shortTTL {
		t.Errorf("TTL after re-join = %v, want > %v (fix3: EXPIRE not called on already-member)", // fix3 — fails if already-member branch skips EXPIRE
			ttlAfterReJoin, shortTTL)
	}
}

func TestRoomRepository_Join_TTL_Expires(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	shortTTL := 100 * time.Millisecond
	client, mr := newTestRedis(t)
	repo := NewRoomRepository(client, shortTTL)

	if _, err := repo.Join(ctx, "room-exp", 1); err != nil {
		t.Fatalf("Join() error = %v", err)
	}

	// Fast-forward past the TTL.
	mr.FastForward(shortTTL + time.Second)

	ok, err := repo.IsMember(ctx, "room-exp", 1)
	if err != nil {
		t.Fatalf("IsMember() error = %v", err)
	}

	if ok {
		t.Error("IsMember() = true after TTL expiry, want false")
	}
}
