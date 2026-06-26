package service_test

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/pizdagladki/full/services/reports/internal/api/repository"
	repomocks "github.com/pizdagladki/full/services/reports/internal/api/repository/mocks"
	"github.com/pizdagladki/full/services/reports/internal/api/service"
)

func TestSessionService_ResolveSession(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		sessionID  string
		setupMock  func(repo *repomocks.MockSessionRepository)
		wantUserID int64
		wantErr    bool
		criterion  string
	}{
		{
			// criterion: 5 — valid session resolves to user ID
			name:      "valid session returns user id",
			sessionID: "good-token",
			setupMock: func(repo *repomocks.MockSessionRepository) {
				repo.EXPECT().UserIDBySession(gomock.Any(), "good-token").Return(int64(42), nil)
			},
			wantUserID: 42,
			criterion:  "AC5",
		},
		{
			// criterion: 5 — session not found propagates ErrSessionNotFound
			name:      "session not found returns ErrSessionNotFound",
			sessionID: "bad-token",
			setupMock: func(repo *repomocks.MockSessionRepository) {
				repo.EXPECT().UserIDBySession(gomock.Any(), "bad-token").
					Return(int64(0), repository.ErrSessionNotFound)
			},
			wantErr:   true,
			criterion: "AC5",
		},
		{
			// criterion: 5 — unexpected repository error propagates
			name:      "repository error propagates",
			sessionID: "err-token",
			setupMock: func(repo *repomocks.MockSessionRepository) {
				repo.EXPECT().UserIDBySession(gomock.Any(), "err-token").
					Return(int64(0), errors.New("redis down"))
			},
			wantErr:   true,
			criterion: "AC5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := repomocks.NewMockSessionRepository(ctrl)
			tt.setupMock(repo)

			svc := service.NewSessionService(repo)
			userID, err := svc.ResolveSession(context.Background(), tt.sessionID)

			if tt.wantErr {
				if err == nil {
					t.Fatal("ResolveSession() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("ResolveSession() unexpected error = %v", err)
			}

			if userID != tt.wantUserID {
				t.Errorf("userID = %d, want %d", userID, tt.wantUserID)
			}
		})
	}
}
