package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/pizdagladki/full/services/reports/internal/api/repository"
)

func TestSessionRepository_UserIDBySession(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		setup        func(mr *miniredis.Miniredis)
		sessionID    string
		wantUserID   int64
		wantErr      bool
		wantNotFound bool
	}{
		{
			// criterion: 5 — valid session returns user ID
			name: "valid session returns user id",
			setup: func(mr *miniredis.Miniredis) {
				mr.Set("session:abc123", "42") //nolint:errcheck
			},
			sessionID:  "abc123",
			wantUserID: 42,
		},
		{
			// criterion: 5 — missing session returns ErrSessionNotFound
			name:         "missing session returns ErrSessionNotFound",
			setup:        func(mr *miniredis.Miniredis) {},
			sessionID:    "nonexistent",
			wantErr:      true,
			wantNotFound: true,
		},
		{
			// criterion: 5 — unparseable user_id value returns error
			name: "invalid user_id value returns error",
			setup: func(mr *miniredis.Miniredis) {
				mr.Set("session:bad", "not-a-number") //nolint:errcheck
			},
			sessionID: "bad",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, mr := newTestRedis(t)
			tt.setup(mr)

			repo := repository.NewSessionRepository(client)
			userID, err := repo.UserIDBySession(context.Background(), tt.sessionID)

			if tt.wantErr {
				if err == nil {
					t.Fatal("UserIDBySession() error = nil, want error")
				}

				if tt.wantNotFound && !errors.Is(err, repository.ErrSessionNotFound) {
					t.Errorf("error = %v, want ErrSessionNotFound", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("UserIDBySession() unexpected error = %v", err)
			}

			if userID != tt.wantUserID {
				t.Errorf("userID = %d, want %d", userID, tt.wantUserID)
			}
		})
	}
}
