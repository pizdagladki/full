package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"

	"github.com/pizdagladki/full/services/media/internal/api/domain"
	"github.com/pizdagladki/full/services/media/internal/api/repository"
)

func clipCols() []string {
	return []string{"id", "user_id", "object_key", "mode", "result", "size_bytes", "content_type", "created_at"}
}

func TestClipRepository_Create(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	cols := clipCols()

	tests := []struct {
		name    string
		clip    domain.Clip
		setup   func(m pgxmock.PgxPoolIface)
		want    domain.Clip
		wantErr bool
	}{
		{
			name: "creates clip and returns populated row",
			clip: domain.Clip{
				UserID:      42,
				ObjectKey:   "clips/42/uuid.webm",
				Mode:        "default",
				Result:      "win",
				ContentType: "video/webm",
				SizeBytes:   1000,
			},
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(cols).
					AddRow(int64(1), int64(42), "clips/42/uuid.webm", "default", "win", int64(1000), "video/webm", now)
				m.ExpectQuery(`INSERT INTO clips`).
					WithArgs(int64(42), "clips/42/uuid.webm", "default", "win", int64(1000), "video/webm").
					WillReturnRows(rows)
			},
			want: domain.Clip{
				ID:          1,
				UserID:      42,
				ObjectKey:   "clips/42/uuid.webm",
				Mode:        "default",
				Result:      "win",
				ContentType: "video/webm",
				SizeBytes:   1000,
				CreatedAt:   now,
			},
		},
		{
			name: "query error propagated",
			clip: domain.Clip{UserID: 1, ObjectKey: "key"},
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`INSERT INTO clips`).WillReturnError(errors.New("db error"))
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

			repo := repository.NewClipRepository(mock)
			got, err := repo.Create(context.Background(), tt.clip)

			if tt.wantErr {
				if err == nil {
					t.Fatal("Create() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("Create() unexpected error = %v", err)
			}

			if got.ID != tt.want.ID {
				t.Errorf("ID = %d, want %d", got.ID, tt.want.ID)
			}

			if got.ObjectKey != tt.want.ObjectKey {
				t.Errorf("ObjectKey = %q, want %q", got.ObjectKey, tt.want.ObjectKey)
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled pgxmock expectations: %v", err)
			}
		})
	}
}

func TestClipRepository_ListByUser(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	cols := clipCols()

	tests := []struct {
		name    string
		userID  int64
		setup   func(m pgxmock.PgxPoolIface)
		wantLen int
		wantErr bool
	}{
		{
			name:   "returns clips ordered newest first",
			userID: 42,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(cols).
					AddRow(int64(3), int64(42), "clips/42/c.webm", "default", "win", int64(300), "video/webm", now).
					AddRow(int64(1), int64(42), "clips/42/a.webm", "default", "win", int64(100), "video/webm", now.Add(-time.Minute))
				m.ExpectQuery(`SELECT`).WithArgs(int64(42)).WillReturnRows(rows)
			},
			wantLen: 2,
		},
		{
			name:   "empty result returns empty slice not nil",
			userID: 99,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(cols)
				m.ExpectQuery(`SELECT`).WithArgs(int64(99)).WillReturnRows(rows)
			},
			wantLen: 0,
		},
		{
			name:   "query error propagated",
			userID: 1,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`SELECT`).WillReturnError(errors.New("connection error"))
			},
			wantErr: true,
		},
		{
			name:   "scan error propagated",
			userID: 1,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"id"}).AddRow(int64(1))
				m.ExpectQuery(`SELECT`).WillReturnRows(rows)
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

			repo := repository.NewClipRepository(mock)
			got, err := repo.ListByUser(context.Background(), tt.userID)

			if tt.wantErr {
				if err == nil {
					t.Fatal("ListByUser() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("ListByUser() unexpected error = %v", err)
			}

			if got == nil {
				t.Fatal("ListByUser() returned nil, want non-nil slice")
			}

			if len(got) != tt.wantLen {
				t.Errorf("len(clips) = %d, want %d", len(got), tt.wantLen)
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled pgxmock expectations: %v", err)
			}
		})
	}
}

func TestClipRepository_GetByID(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	cols := clipCols()

	tests := []struct {
		name    string
		id      int64
		setup   func(m pgxmock.PgxPoolIface)
		wantID  int64
		wantErr error
	}{
		{
			name: "returns clip when found",
			id:   1,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(cols).
					AddRow(int64(1), int64(42), "clips/42/a.webm", "default", "win", int64(500), "video/webm", now)
				m.ExpectQuery(`SELECT`).WithArgs(int64(1)).WillReturnRows(rows)
			},
			wantID: 1,
		},
		{
			name: "returns ErrClipNotFound when no rows",
			id:   999,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(cols)
				m.ExpectQuery(`SELECT`).WithArgs(int64(999)).WillReturnRows(rows)
			},
			wantErr: repository.ErrClipNotFound,
		},
		{
			name: "returns error on query failure",
			id:   1,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`SELECT`).WillReturnError(errors.New("db error"))
			},
			wantErr: errors.New("db error"),
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

			repo := repository.NewClipRepository(mock)
			got, err := repo.GetByID(context.Background(), tt.id)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("GetByID() error = nil, want error")
				}

				if errors.Is(tt.wantErr, repository.ErrClipNotFound) && !errors.Is(err, repository.ErrClipNotFound) {
					t.Errorf("GetByID() error = %v, want ErrClipNotFound", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("GetByID() unexpected error = %v", err)
			}

			if got.ID != tt.wantID {
				t.Errorf("ID = %d, want %d", got.ID, tt.wantID)
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled pgxmock expectations: %v", err)
			}
		})
	}
}

func TestClipRepository_DeleteOldestBeyondLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		userID   int64
		limit    int
		setup    func(m pgxmock.PgxPoolIface)
		wantKeys []string
		wantErr  bool
	}{
		{
			name:   "returns evicted object keys when over limit",
			userID: 42,
			limit:  10,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"object_key"}).
					AddRow("clips/42/oldest.webm")
				m.ExpectQuery(`DELETE FROM clips`).
					WithArgs(int64(42), 10).
					WillReturnRows(rows)
			},
			wantKeys: []string{"clips/42/oldest.webm"},
		},
		{
			name:   "returns empty slice when nothing to evict",
			userID: 42,
			limit:  10,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"object_key"})
				m.ExpectQuery(`DELETE FROM clips`).
					WithArgs(int64(42), 10).
					WillReturnRows(rows)
			},
			wantKeys: []string{},
		},
		{
			name:   "query error propagated",
			userID: 1,
			limit:  10,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`DELETE FROM clips`).WillReturnError(errors.New("db error"))
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

			repo := repository.NewClipRepository(mock)
			keys, err := repo.DeleteOldestBeyondLimit(context.Background(), tt.userID, tt.limit)

			if tt.wantErr {
				if err == nil {
					t.Fatal("DeleteOldestBeyondLimit() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("DeleteOldestBeyondLimit() unexpected error = %v", err)
			}

			if len(keys) != len(tt.wantKeys) {
				t.Errorf("len(keys) = %d, want %d", len(keys), len(tt.wantKeys))
			}

			for i, k := range keys {
				if k != tt.wantKeys[i] {
					t.Errorf("keys[%d] = %q, want %q", i, k, tt.wantKeys[i])
				}
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled pgxmock expectations: %v", err)
			}
		})
	}
}
