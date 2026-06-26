package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	return client, mr
}

func TestSessionRepository_UserIDBySession(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setup      func(mr *miniredis.Miniredis)
		sessionID  string
		wantID     int64
		wantErr    error
		wantAnyErr bool
	}{
		{
			name: "valid session returns user id", // criterion: 1
			setup: func(mr *miniredis.Miniredis) {
				mr.Set("session:abc123", "42")
			},
			sessionID: "abc123",
			wantID:    42,
		},
		{
			name:      "missing session returns ErrSessionNotFound", // criterion: 1 — fails when absent session not rejected
			setup:     func(_ *miniredis.Miniredis) {},
			sessionID: "nonexistent",
			wantErr:   ErrSessionNotFound,
		},
		{
			name: "expired session returns ErrSessionNotFound", // criterion: 1 — fails when expired session not rejected
			setup: func(mr *miniredis.Miniredis) {
				mr.Set("session:expiring", "99")
				mr.SetTTL("session:expiring", 1*time.Millisecond)
				mr.FastForward(time.Second)
			},
			sessionID: "expiring",
			wantErr:   ErrSessionNotFound,
		},
		{
			name: "non-numeric value returns parse error",
			setup: func(mr *miniredis.Miniredis) {
				mr.Set("session:bad", "not-a-number")
			},
			sessionID:  "bad",
			wantAnyErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, mr := newTestRedis(t)
			tt.setup(mr)

			repo := NewSessionRepository(client)
			got, err := repo.UserIDBySession(context.Background(), tt.sessionID)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("UserIDBySession() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if tt.wantAnyErr {
				if err == nil {
					t.Fatal("UserIDBySession() error = nil, want parse error")
				}

				return
			}

			if err != nil {
				t.Fatalf("UserIDBySession() unexpected error = %v", err)
			}

			if got != tt.wantID {
				t.Errorf("UserIDBySession() = %d, want %d", got, tt.wantID)
			}
		})
	}
}
