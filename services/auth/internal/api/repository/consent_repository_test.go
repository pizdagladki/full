package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"

	"github.com/pizdagladki/full/services/auth/internal/api/domain"
	"github.com/pizdagladki/full/services/auth/internal/api/repository"
)

func TestConsentRepository_Upsert(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name    string
		userID  int64
		input   domain.Consent
		setup   func(m pgxmock.PgxPoolIface)
		wantErr bool
	}{
		{
			// Criterion 1 + 4: first insert upserts and returns row.
			name:   "first insert returns consent row",
			userID: 1,
			input:  domain.Consent{IsAdult: true, ConsentRecording: true, ConsentTos: true},
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"is_adult", "consent_recording", "consent_tos", "accepted_at"}).
					AddRow(true, true, true, now)
				m.ExpectQuery(`INSERT INTO user_consents`).
					WithArgs(int64(1), true, true, true).
					WillReturnRows(rows)
			},
		},
		{
			// Criterion 4: second submit for same user updates the single row.
			name:   "conflict update returns updated row",
			userID: 1,
			input:  domain.Consent{IsAdult: true, ConsentRecording: true, ConsentTos: true},
			setup: func(m pgxmock.PgxPoolIface) {
				// ON CONFLICT path: same row returned with fresh accepted_at
				rows := pgxmock.NewRows([]string{"is_adult", "consent_recording", "consent_tos", "accepted_at"}).
					AddRow(true, true, true, now)
				m.ExpectQuery(`INSERT INTO user_consents`).
					WithArgs(int64(1), true, true, true).
					WillReturnRows(rows)
			},
		},
		{
			name:   "db error propagated",
			userID: 99,
			input:  domain.Consent{IsAdult: true, ConsentRecording: true, ConsentTos: true},
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`INSERT INTO user_consents`).
					WithArgs(int64(99), true, true, true).
					WillReturnError(errors.New("connection refused"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock, err := pgxmock.NewPool()
			if err != nil {
				t.Fatalf("pgxmock.NewPool() error = %v", err)
			}
			defer mock.Close()

			tt.setup(mock)

			repo := repository.NewConsentRepository(mock)

			result, err := repo.Upsert(context.Background(), tt.userID, tt.input)

			if tt.wantErr {
				if err == nil {
					t.Fatal("Upsert() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("Upsert() unexpected error = %v", err)
			}
			if !result.IsAdult {
				t.Error("result.IsAdult = false, want true")
			}
			if !result.ConsentRecording {
				t.Error("result.ConsentRecording = false, want true")
			}
			if !result.ConsentTos {
				t.Error("result.ConsentTos = false, want true")
			}
			if result.AcceptedAt.IsZero() {
				t.Error("result.AcceptedAt is zero, want a non-zero timestamp")
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled pgxmock expectations: %v", err)
			}
		})
	}
}

func TestConsentRepository_GetByUserID(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name      string
		userID    int64
		setup     func(m pgxmock.PgxPoolIface)
		wantErr   error
		wantFound bool
	}{
		{
			// Criterion 5: found row returned.
			name:   "found",
			userID: 42,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"is_adult", "consent_recording", "consent_tos", "accepted_at"}).
					AddRow(true, true, true, now)
				m.ExpectQuery(`SELECT`).WithArgs(int64(42)).WillReturnRows(rows)
			},
			wantFound: true,
		},
		{
			// No row → ErrConsentNotFound sentinel.
			name:   "not found returns ErrConsentNotFound",
			userID: 999,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`SELECT`).WithArgs(int64(999)).
					WillReturnRows(pgxmock.NewRows([]string{"is_adult", "consent_recording", "consent_tos", "accepted_at"}))
			},
			wantErr: repository.ErrConsentNotFound,
		},
		{
			// Other db error propagated as-is.
			name:   "db error propagated",
			userID: 1,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`SELECT`).WithArgs(int64(1)).
					WillReturnError(errors.New("timeout"))
			},
			wantErr: errors.New("timeout"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock, err := pgxmock.NewPool()
			if err != nil {
				t.Fatalf("pgxmock.NewPool() error = %v", err)
			}
			defer mock.Close()

			tt.setup(mock)

			repo := repository.NewConsentRepository(mock)

			consent, err := repo.GetByUserID(context.Background(), tt.userID)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("GetByUserID() error = nil, want %v", tt.wantErr)
				}
				if errors.Is(tt.wantErr, repository.ErrConsentNotFound) && !errors.Is(err, repository.ErrConsentNotFound) {
					t.Errorf("GetByUserID() error = %v, want ErrConsentNotFound", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("GetByUserID() unexpected error = %v", err)
			}
			if !tt.wantFound {
				t.Fatal("test case marked wantFound=false but wantErr=nil — logic error in test table")
			}
			if !consent.IsAdult {
				t.Error("consent.IsAdult = false, want true")
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled pgxmock expectations: %v", err)
			}
		})
	}
}
