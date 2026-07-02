package repository_test

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/pizdagladki/full/services/store/internal/api/repository"
)

func TestPointsCache_GetBalance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		userID     int64
		setup      func(mr *miniredis.Miniredis)
		wantFound  bool
		wantVal    int64
		wantErr    bool
		wantErrMsg string
	}{
		{
			// criterion: 3 — cache hit returns the cached balance with found=true.
			name:   "cache hit returns balance",
			userID: 1,
			setup: func(mr *miniredis.Miniredis) {
				if err := mr.Set("points:balance:1", "42"); err != nil {
					t.Fatalf("miniredis Set: %v", err)
				}
			},
			wantFound: true,
			wantVal:   42,
		},
		{
			// criterion: 3 — cache miss returns found=false, no error (falls through
			// to Postgres at the service layer).
			name:      "cache miss returns found false no error",
			userID:    2,
			setup:     func(_ *miniredis.Miniredis) {},
			wantFound: false,
			wantVal:   0,
		},
		{
			name:   "malformed cached value returns parse error",
			userID: 3,
			setup: func(mr *miniredis.Miniredis) {
				if err := mr.Set("points:balance:3", "not-a-number"); err != nil {
					t.Fatalf("miniredis Set: %v", err)
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mr := miniredis.RunT(t)
			tt.setup(mr)

			client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			defer client.Close()

			cache := repository.NewPointsCache(client)
			got, found, err := cache.GetBalance(context.Background(), tt.userID)

			if tt.wantErr {
				if err == nil {
					t.Fatal("GetBalance() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("GetBalance() unexpected error = %v", err)
			}

			if found != tt.wantFound {
				t.Errorf("GetBalance() found = %v, want %v", found, tt.wantFound)
			}

			if got != tt.wantVal {
				t.Errorf("GetBalance() = %d, want %d", got, tt.wantVal)
			}
		})
	}
}

func TestPointsCache_SetBalance(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	cache := repository.NewPointsCache(client)

	err := cache.SetBalance(context.Background(), 10, 99)
	if err != nil {
		t.Fatalf("SetBalance() unexpected error = %v", err)
	}

	val, err := mr.Get("points:balance:10")
	if err != nil {
		t.Fatalf("miniredis Get: %v", err)
	}

	if val != "99" {
		t.Errorf("stored value = %q, want %q", val, "99")
	}
}

func TestPointsCache_SetBalance_ClosedClientErrors(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	_ = client.Close()

	cache := repository.NewPointsCache(client)

	err := cache.SetBalance(context.Background(), 1, 1)
	if err == nil {
		t.Fatal("SetBalance() error = nil, want error against a closed client")
	}
}

func TestPointsCache_DeleteBalance(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	cache := repository.NewPointsCache(client)

	if err := mr.Set("points:balance:20", "5"); err != nil {
		t.Fatalf("miniredis Set: %v", err)
	}

	err := cache.DeleteBalance(context.Background(), 20)
	if err != nil {
		t.Fatalf("DeleteBalance() unexpected error = %v", err)
	}

	if mr.Exists("points:balance:20") {
		t.Error("key still exists after DeleteBalance")
	}
}

func TestPointsCache_DeleteBalance_ClosedClientErrors(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	_ = client.Close()

	cache := repository.NewPointsCache(client)

	err := cache.DeleteBalance(context.Background(), 1)
	if err == nil {
		t.Fatal("DeleteBalance() error = nil, want error against a closed client")
	}
}
