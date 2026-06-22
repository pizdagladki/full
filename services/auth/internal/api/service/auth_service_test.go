package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/auth/internal/api/domain"
	repomocks "github.com/pizdagladki/full/services/auth/internal/api/repository/mocks"
	"github.com/pizdagladki/full/services/auth/internal/api/service"
	svcmocks "github.com/pizdagladki/full/services/auth/internal/api/service/mocks"
)

func TestAuthService_LoginGoogle(t *testing.T) {
	t.Parallel()

	fixedUser := domain.User{ID: 1, GoogleSub: "sub-123", Email: "alice@example.com", CreatedAt: time.Now()}

	tests := []struct {
		name          string
		code          string
		setupOAuth    func(m *svcmocks.MockOAuthExchanger)
		setupRepo     func(m *repomocks.MockUserRepository)
		setupSession  func(m *svcmocks.MockSessionStore)
		wantSessionID string
		wantUser      domain.User
		wantErr       bool
		wantErrIs     error // optional: checked via errors.Is when non-nil
	}{
		{
			name: "happy path - first login creates user and session",
			code: "valid-code",
			setupOAuth: func(m *svcmocks.MockOAuthExchanger) {
				m.EXPECT().ExchangeCode(gomock.Any(), "valid-code").
					Return(service.GoogleUser{Sub: "sub-123", Email: "alice@example.com"}, nil)
			},
			setupRepo: func(m *repomocks.MockUserRepository) {
				m.EXPECT().UpsertByGoogleSub(gomock.Any(), "sub-123", "alice@example.com").
					Return(fixedUser, nil)
			},
			setupSession: func(m *svcmocks.MockSessionStore) {
				m.EXPECT().Create(gomock.Any(), int64(1)).Return("sess-abc", nil)
			},
			wantSessionID: "sess-abc",
			wantUser:      fixedUser,
		},
		{
			name: "repeat login same Google account - upsert called exactly once",
			code: "valid-code-2",
			setupOAuth: func(m *svcmocks.MockOAuthExchanger) {
				m.EXPECT().ExchangeCode(gomock.Any(), "valid-code-2").
					Return(service.GoogleUser{Sub: "sub-123", Email: "alice@example.com"}, nil)
			},
			setupRepo: func(m *repomocks.MockUserRepository) {
				m.EXPECT().UpsertByGoogleSub(gomock.Any(), "sub-123", "alice@example.com").
					Times(1).Return(fixedUser, nil)
			},
			setupSession: func(m *svcmocks.MockSessionStore) {
				m.EXPECT().Create(gomock.Any(), int64(1)).Return("sess-xyz", nil)
			},
			wantSessionID: "sess-xyz",
			wantUser:      fixedUser,
		},
		{
			name: "invalid code - returns ErrInvalidCode, no session created",
			code: "bad-code",
			setupOAuth: func(m *svcmocks.MockOAuthExchanger) {
				m.EXPECT().ExchangeCode(gomock.Any(), "bad-code").
					Return(service.GoogleUser{}, service.ErrInvalidCode)
			},
			setupRepo:    func(_ *repomocks.MockUserRepository) {},
			setupSession: func(_ *svcmocks.MockSessionStore) {},
			wantErr:      true,
			wantErrIs:    service.ErrInvalidCode,
		},
		{
			name: "repo error propagated",
			code: "valid-code-3",
			setupOAuth: func(m *svcmocks.MockOAuthExchanger) {
				m.EXPECT().ExchangeCode(gomock.Any(), "valid-code-3").
					Return(service.GoogleUser{Sub: "sub-999", Email: "bob@example.com"}, nil)
			},
			setupRepo: func(m *repomocks.MockUserRepository) {
				m.EXPECT().UpsertByGoogleSub(gomock.Any(), "sub-999", "bob@example.com").
					Return(domain.User{}, errors.New("db error"))
			},
			setupSession: func(_ *svcmocks.MockSessionStore) {},
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			oauthMock := svcmocks.NewMockOAuthExchanger(ctrl)
			repoMock := repomocks.NewMockUserRepository(ctrl)
			sessionMock := svcmocks.NewMockSessionStore(ctrl)

			tt.setupOAuth(oauthMock)
			tt.setupRepo(repoMock)
			tt.setupSession(sessionMock)

			svc := service.NewAuthService(repoMock, oauthMock, sessionMock, zap.NewNop())

			sessionID, user, err := svc.LoginGoogle(context.Background(), tt.code)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("LoginGoogle() error = nil, want error")
				}

				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Errorf("LoginGoogle() error = %v, want errors.Is(%v)", err, tt.wantErrIs)
				}

				if sessionID != "" {
					t.Errorf("LoginGoogle() sessionID = %q, want empty on error", sessionID)
				}

				return
			}

			if err != nil {
				t.Fatalf("LoginGoogle() unexpected error = %v", err)
			}
			if sessionID != tt.wantSessionID {
				t.Errorf("LoginGoogle() sessionID = %q, want %q", sessionID, tt.wantSessionID)
			}
			if user.ID != tt.wantUser.ID || user.Email != tt.wantUser.Email {
				t.Errorf("LoginGoogle() user = %+v, want %+v", user, tt.wantUser)
			}
		})
	}
}

func TestAuthService_Authenticate(t *testing.T) {
	t.Parallel()

	fixedUser := domain.User{ID: 42, GoogleSub: "sub-42", Email: "carol@example.com"}

	tests := []struct {
		name         string
		sessionID    string
		setupSession func(m *svcmocks.MockSessionStore)
		setupRepo    func(m *repomocks.MockUserRepository)
		wantUser     domain.User
		wantErr      bool
		wantErrIs    error
	}{
		{
			name:      "happy path - valid session returns user",
			sessionID: "valid-sess",
			setupSession: func(m *svcmocks.MockSessionStore) {
				m.EXPECT().Get(gomock.Any(), "valid-sess").Return(int64(42), nil)
			},
			setupRepo: func(m *repomocks.MockUserRepository) {
				m.EXPECT().GetByID(gomock.Any(), int64(42)).Return(fixedUser, nil)
			},
			wantUser: fixedUser,
		},
		{
			name:      "expired/missing session - ErrSessionNotFound",
			sessionID: "stale-sess",
			setupSession: func(m *svcmocks.MockSessionStore) {
				m.EXPECT().Get(gomock.Any(), "stale-sess").Return(int64(0), service.ErrSessionNotFound)
			},
			setupRepo: func(_ *repomocks.MockUserRepository) {},
			wantErr:   true,
			wantErrIs: service.ErrSessionNotFound,
		},
		{
			name:      "user deleted from db - repo error",
			sessionID: "orphan-sess",
			setupSession: func(m *svcmocks.MockSessionStore) {
				m.EXPECT().Get(gomock.Any(), "orphan-sess").Return(int64(99), nil)
			},
			setupRepo: func(m *repomocks.MockUserRepository) {
				m.EXPECT().GetByID(gomock.Any(), int64(99)).Return(domain.User{}, errors.New("not found"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			sessionMock := svcmocks.NewMockSessionStore(ctrl)
			repoMock := repomocks.NewMockUserRepository(ctrl)
			oauthMock := svcmocks.NewMockOAuthExchanger(ctrl)

			tt.setupSession(sessionMock)
			tt.setupRepo(repoMock)

			svc := service.NewAuthService(repoMock, oauthMock, sessionMock, zap.NewNop())

			user, err := svc.Authenticate(context.Background(), tt.sessionID)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("Authenticate() error = nil, want error")
				}

				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Errorf("Authenticate() error = %v, want errors.Is(%v)", err, tt.wantErrIs)
				}

				return
			}

			if err != nil {
				t.Fatalf("Authenticate() unexpected error = %v", err)
			}
			if user.ID != tt.wantUser.ID || user.Email != tt.wantUser.Email {
				t.Errorf("Authenticate() user = %+v, want %+v", user, tt.wantUser)
			}
		})
	}
}
