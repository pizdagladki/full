package service_test

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/pizdagladki/full/services/media/internal/api/repository"
	repomocks "github.com/pizdagladki/full/services/media/internal/api/repository/mocks"
	"github.com/pizdagladki/full/services/media/internal/api/service"
)

func TestSessionService_ResolveSession(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sessionID string
		setupRepo func(m *repomocks.MockSessionRepository)
		wantID    int64
		wantErr   error
	}{
		{
			name:      "valid session returns user id",
			sessionID: "valid-session",
			setupRepo: func(m *repomocks.MockSessionRepository) {
				m.EXPECT().UserIDBySession(gomock.Any(), "valid-session").Return(int64(42), nil)
			},
			wantID: 42,
		},
		{
			name:      "missing session returns ErrSessionNotFound",
			sessionID: "missing",
			setupRepo: func(m *repomocks.MockSessionRepository) {
				m.EXPECT().UserIDBySession(gomock.Any(), "missing").Return(int64(0), repository.ErrSessionNotFound)
			},
			wantErr: repository.ErrSessionNotFound,
		},
		{
			name:      "repo error propagated",
			sessionID: "broken",
			setupRepo: func(m *repomocks.MockSessionRepository) {
				m.EXPECT().UserIDBySession(gomock.Any(), "broken").Return(int64(0), errors.New("redis timeout"))
			},
			wantErr: errors.New("redis timeout"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repoMock := repomocks.NewMockSessionRepository(ctrl)
			tt.setupRepo(repoMock)

			svc := service.NewSessionService(repoMock)

			got, err := svc.ResolveSession(context.Background(), tt.sessionID)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("ResolveSession() error = nil, want error")
				}

				if errors.Is(tt.wantErr, repository.ErrSessionNotFound) && !errors.Is(err, repository.ErrSessionNotFound) {
					t.Errorf("ResolveSession() error = %v, want ErrSessionNotFound", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("ResolveSession() unexpected error = %v", err)
			}

			if got != tt.wantID {
				t.Errorf("userID = %d, want %d", got, tt.wantID)
			}
		})
	}
}
