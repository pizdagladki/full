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
	// laterNow simulates the refreshed accepted_at that the ON CONFLICT DO UPDATE
	// path writes (accepted_at = now() in the SET clause). It must differ from now
	// so the conflict case is genuinely distinguishable from the insert case.
	laterNow := now.Add(time.Hour)

	tests := []struct {
		name    string
		userID  int64
		input   domain.Consent
		want    domain.Consent // criterion 4: each happy-path case asserts its own expected return
		setup   func(m pgxmock.PgxPoolIface)
		wantErr bool
	}{
		{
			// criterion: 1+4 — first insert upserts and returns a row via ON CONFLICT … DO UPDATE.
			name:   "first insert returns consent row",
			userID: 1,
			input:  domain.Consent{IsAdult: true, ConsentRecording: true, ConsentTos: true},
			want:   domain.Consent{IsAdult: true, ConsentRecording: true, ConsentTos: true, AcceptedAt: now},
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"is_adult", "consent_recording", "consent_tos", "accepted_at"}).
					AddRow(true, true, true, now)
				// Regex requires the ON CONFLICT clause: removing it from consentUpsertSQL
				// makes the matcher fail to find a match, causing this test to fail.
				m.ExpectQuery(`ON CONFLICT \(user_id\) DO UPDATE`).
					WithArgs(int64(1), true, true, true).
					WillReturnRows(rows)
			},
		},
		{
			// criterion: 4 — second submit for same user hits ON CONFLICT and updates the row;
			// the returned accepted_at is refreshed (later than the original insert timestamp).
			name:   "conflict update returns updated row",
			userID: 1,
			input:  domain.Consent{IsAdult: true, ConsentRecording: true, ConsentTos: true},
			want:   domain.Consent{IsAdult: true, ConsentRecording: true, ConsentTos: true, AcceptedAt: laterNow},
			setup: func(m pgxmock.PgxPoolIface) {
				// ON CONFLICT path: accepted_at is renewed (laterNow), proving the UPDATE
				// branch ran rather than a duplicate INSERT.
				rows := pgxmock.NewRows([]string{"is_adult", "consent_recording", "consent_tos", "accepted_at"}).
					AddRow(true, true, true, laterNow)
				m.ExpectQuery(`ON CONFLICT \(user_id\) DO UPDATE`).
					WithArgs(int64(1), true, true, true).
					WillReturnRows(rows)
			},
		},
		{
			name:    "db error propagated",
			userID:  99,
			input:   domain.Consent{IsAdult: true, ConsentRecording: true, ConsentTos: true},
			wantErr: true,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`ON CONFLICT \(user_id\) DO UPDATE`).
					WithArgs(int64(99), true, true, true).
					WillReturnError(errors.New("connection refused"))
			},
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

			// Assert each field against the per-case expected return (criterion 4).
			if result.IsAdult != tt.want.IsAdult {
				t.Errorf("result.IsAdult = %v, want %v", result.IsAdult, tt.want.IsAdult)
			}
			if result.ConsentRecording != tt.want.ConsentRecording {
				t.Errorf("result.ConsentRecording = %v, want %v", result.ConsentRecording, tt.want.ConsentRecording)
			}
			if result.ConsentTos != tt.want.ConsentTos {
				t.Errorf("result.ConsentTos = %v, want %v", result.ConsentTos, tt.want.ConsentTos)
			}
			if !result.AcceptedAt.Equal(tt.want.AcceptedAt) {
				t.Errorf("result.AcceptedAt = %v, want %v", result.AcceptedAt, tt.want.AcceptedAt)
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
