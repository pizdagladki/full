package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/auth/internal/api/domain"
	"github.com/pizdagladki/full/services/auth/internal/api/repository"
	repomocks "github.com/pizdagladki/full/services/auth/internal/api/repository/mocks"
	"github.com/pizdagladki/full/services/auth/internal/api/service"
)

func TestConsentService_RecordConsent(t *testing.T) {
	t.Parallel()

	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	req := domain.ConsentRequest{IsAdult: true, ConsentRecording: true, ConsentTos: true}
	expectedConsent := domain.Consent{
		IsAdult:          true,
		ConsentRecording: true,
		ConsentTos:       true,
		AcceptedAt:       fixedTime,
	}

	tests := []struct {
		name      string
		userID    int64
		req       domain.ConsentRequest
		setupRepo func(m *repomocks.MockConsentRepository)
		wantErr   bool
	}{
		{
			// Criterion 1: happy path → returns upserted consent.
			name:   "happy path - returns upserted consent",
			userID: 42,
			req:    req,
			setupRepo: func(m *repomocks.MockConsentRepository) {
				m.EXPECT().Upsert(gomock.Any(), int64(42), domain.Consent{
					IsAdult:          true,
					ConsentRecording: true,
					ConsentTos:       true,
				}).Return(expectedConsent, nil)
			},
		},
		{
			// Repo error propagated.
			name:   "repo error propagated",
			userID: 42,
			req:    req,
			setupRepo: func(m *repomocks.MockConsentRepository) {
				m.EXPECT().Upsert(gomock.Any(), int64(42), gomock.Any()).
					Return(domain.Consent{}, errors.New("db error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repoMock := repomocks.NewMockConsentRepository(ctrl)
			tt.setupRepo(repoMock)

			svc := service.NewConsentService(repoMock, zap.NewNop())

			result, err := svc.RecordConsent(context.Background(), tt.userID, tt.req)

			if tt.wantErr {
				if err == nil {
					t.Fatal("RecordConsent() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("RecordConsent() unexpected error = %v", err)
			}
			if result.AcceptedAt != expectedConsent.AcceptedAt {
				t.Errorf("result.AcceptedAt = %v, want %v", result.AcceptedAt, expectedConsent.AcceptedAt)
			}
		})
	}
}

func TestConsentService_GetConsent(t *testing.T) {
	t.Parallel()

	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	storedConsent := domain.Consent{
		IsAdult:          true,
		ConsentRecording: true,
		ConsentTos:       true,
		AcceptedAt:       fixedTime,
	}

	tests := []struct {
		name        string
		userID      int64
		setupRepo   func(m *repomocks.MockConsentRepository)
		wantConsent *domain.Consent
		wantErr     bool
	}{
		{
			// Criterion 5: consent found → returned as non-nil pointer.
			name:   "consent found - returns pointer",
			userID: 42,
			setupRepo: func(m *repomocks.MockConsentRepository) {
				m.EXPECT().GetByUserID(gomock.Any(), int64(42)).Return(storedConsent, nil)
			},
			wantConsent: &storedConsent,
		},
		{
			// Criterion 5: ErrConsentNotFound → (nil, nil).
			name:   "not found - returns nil nil",
			userID: 99,
			setupRepo: func(m *repomocks.MockConsentRepository) {
				m.EXPECT().GetByUserID(gomock.Any(), int64(99)).
					Return(domain.Consent{}, repository.ErrConsentNotFound)
			},
			wantConsent: nil,
		},
		{
			// Other errors propagated.
			name:   "db error propagated",
			userID: 1,
			setupRepo: func(m *repomocks.MockConsentRepository) {
				m.EXPECT().GetByUserID(gomock.Any(), int64(1)).
					Return(domain.Consent{}, errors.New("db error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repoMock := repomocks.NewMockConsentRepository(ctrl)
			tt.setupRepo(repoMock)

			svc := service.NewConsentService(repoMock, zap.NewNop())

			result, err := svc.GetConsent(context.Background(), tt.userID)

			if tt.wantErr {
				if err == nil {
					t.Fatal("GetConsent() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("GetConsent() unexpected error = %v", err)
			}

			if tt.wantConsent == nil {
				if result != nil {
					t.Errorf("GetConsent() result = %+v, want nil", result)
				}

				return
			}

			if result == nil {
				t.Fatal("GetConsent() result = nil, want non-nil")
			}
			if result.AcceptedAt != tt.wantConsent.AcceptedAt {
				t.Errorf("result.AcceptedAt = %v, want %v", result.AcceptedAt, tt.wantConsent.AcceptedAt)
			}
		})
	}
}
