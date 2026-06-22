package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"

	"github.com/pizdagladki/full/services/auth/internal/api/repository"
)

func TestUserRepository_UpsertByGoogleSub(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name      string
		googleSub string
		email     string
		setup     func(m pgxmock.PgxPoolIface)
		wantID    int64
		wantErr   bool
	}{
		{
			name:      "first insert returns new user",
			googleSub: "sub-111",
			email:     "alice@example.com",
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"id", "google_sub", "email", "created_at"}).
					AddRow(int64(1), "sub-111", "alice@example.com", now)
				m.ExpectQuery(`INSERT INTO users`).
					WithArgs("sub-111", "alice@example.com").
					WillReturnRows(rows)
			},
			wantID: 1,
		},
		{
			name:      "repeat login does not create second row (conflict update)",
			googleSub: "sub-111",
			email:     "alice@example.com",
			setup: func(m pgxmock.PgxPoolIface) {
				// ON CONFLICT path: same id returned, email updated
				rows := pgxmock.NewRows([]string{"id", "google_sub", "email", "created_at"}).
					AddRow(int64(1), "sub-111", "alice@example.com", now)
				m.ExpectQuery(`INSERT INTO users`).
					WithArgs("sub-111", "alice@example.com").
					WillReturnRows(rows)
			},
			wantID: 1,
		},
		{
			name:      "db error propagated",
			googleSub: "sub-err",
			email:     "err@example.com",
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`INSERT INTO users`).
					WithArgs("sub-err", "err@example.com").
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

			repo := repository.NewUserRepository(mock)

			user, err := repo.UpsertByGoogleSub(context.Background(), tt.googleSub, tt.email)

			if tt.wantErr {
				if err == nil {
					t.Fatal("UpsertByGoogleSub() error = nil, want error")
				}

				return
			}
			if err != nil {
				t.Fatalf("UpsertByGoogleSub() unexpected error = %v", err)
			}
			if user.ID != tt.wantID {
				t.Errorf("user.ID = %d, want %d", user.ID, tt.wantID)
			}
			if user.Email != tt.email {
				t.Errorf("user.Email = %q, want %q", user.Email, tt.email)
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled pgxmock expectations: %v", err)
			}
		})
	}
}

func TestUserRepository_GetByID(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name    string
		id      int64
		setup   func(m pgxmock.PgxPoolIface)
		wantErr error
	}{
		{
			name: "found",
			id:   42,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"id", "google_sub", "email", "created_at"}).
					AddRow(int64(42), "sub-42", "carol@example.com", now)
				m.ExpectQuery(`SELECT`).WithArgs(int64(42)).WillReturnRows(rows)
			},
		},
		{
			name: "not found returns ErrNotFound",
			id:   999,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`SELECT`).WithArgs(int64(999)).
					WillReturnRows(pgxmock.NewRows([]string{"id", "google_sub", "email", "created_at"}))
			},
			wantErr: repository.ErrNotFound,
		},
		{
			name: "db error propagated",
			id:   1,
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

			repo := repository.NewUserRepository(mock)

			user, err := repo.GetByID(context.Background(), tt.id)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("GetByID() error = nil, want %v", tt.wantErr)
				}
				if errors.Is(tt.wantErr, repository.ErrNotFound) && !errors.Is(err, repository.ErrNotFound) {
					t.Errorf("GetByID() error = %v, want ErrNotFound", err)
				}

				return
			}
			if err != nil {
				t.Fatalf("GetByID() unexpected error = %v", err)
			}
			if user.ID != tt.id {
				t.Errorf("user.ID = %d, want %d", user.ID, tt.id)
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled pgxmock expectations: %v", err)
			}
		})
	}
}
