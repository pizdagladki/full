package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/pizdagladki/full/services/media/internal/api/repository"
)

func TestSessionRepository_UserIDBySession(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(mr *miniredis.Miniredis)
		sessionID string
		wantID    int64
		wantErr   error
	}{
		{
			name: "valid session returns user id",
			setup: func(mr *miniredis.Miniredis) {
				if err := mr.Set("session:abc123", "42"); err != nil {
					t.Fatalf("miniredis Set: %v", err)
				}
			},
			sessionID: "abc123",
			wantID:    42,
		},
		{
			name:      "missing session returns ErrSessionNotFound",
			setup:     func(_ *miniredis.Miniredis) {},
			sessionID: "missing",
			wantErr:   repository.ErrSessionNotFound,
		},
		{
			name: "malformed value returns parse error",
			setup: func(mr *miniredis.Miniredis) {
				if err := mr.Set("session:bad", "not-a-number"); err != nil {
					t.Fatalf("miniredis Set: %v", err)
				}
			},
			sessionID: "bad",
			wantErr:   errors.New("parse error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mr := miniredis.RunT(t)
			tt.setup(mr)

			client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			defer client.Close()

			repo := repository.NewSessionRepository(client)

			got, err := repo.UserIDBySession(context.Background(), tt.sessionID)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("UserIDBySession() error = nil, want %v", tt.wantErr)
				}

				if errors.Is(tt.wantErr, repository.ErrSessionNotFound) && !errors.Is(err, repository.ErrSessionNotFound) {
					t.Errorf("UserIDBySession() error = %v, want ErrSessionNotFound", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("UserIDBySession() unexpected error = %v", err)
			}

			if got != tt.wantID {
				t.Errorf("userID = %d, want %d", got, tt.wantID)
			}
		})
	}
}
