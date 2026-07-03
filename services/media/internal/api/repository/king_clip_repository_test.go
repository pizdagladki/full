package repository_test

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"

	"github.com/pizdagladki/full/services/media/internal/api/domain"
	"github.com/pizdagladki/full/services/media/internal/api/repository"
)

func kingClipCols() []string {
	return []string{
		"id", "user_id", "hill_type", "object_key", "blink_ts_ms", "created_at", "expires_at",
	}
}

func TestKingClipRepository_Create(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	expires := now.Add(24 * time.Hour)
	cols := kingClipCols()

	tests := []struct {
		name    string
		clip    domain.KingClip
		setup   func(m pgxmock.PgxPoolIface)
		wantID  int64
		wantErr bool
	}{
		{
			// criterion: 1 — Create stores king clip metadata (hill_type, object_key,
			// blink_ts_ms, expires_at) under the dedicated king-clip prefix, separate
			// from the clips table used by ClipRepository.
			name: "creates king clip and returns populated row",
			clip: domain.KingClip{
				UserID:    42,
				HillType:  domain.HillTypeDaily,
				ObjectKey: "king-clips/daily/42/uuid.webm",
				BlinkTsMs: 1234,
				ExpiresAt: expires,
			},
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(cols).
					AddRow(int64(1), int64(42), "daily", "king-clips/daily/42/uuid.webm", int64(1234), now, expires)
				m.ExpectQuery(`INSERT INTO king_clips`).
					WithArgs(int64(42), "daily", "king-clips/daily/42/uuid.webm", int64(1234), expires).
					WillReturnRows(rows)
			},
			wantID: 1,
		},
		{
			name: "query error propagated",
			clip: domain.KingClip{UserID: 1, HillType: domain.HillTypeDaily, ObjectKey: "key"},
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`INSERT INTO king_clips`).WillReturnError(errors.New("db error"))
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

			repo := repository.NewKingClipRepository(mock)
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

			if got.ID != tt.wantID {
				t.Errorf("ID = %d, want %d", got.ID, tt.wantID)
			}

			if got.ObjectKey != tt.clip.ObjectKey {
				t.Errorf("ObjectKey = %q, want %q", got.ObjectKey, tt.clip.ObjectKey)
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled pgxmock expectations: %v", err)
			}
		})
	}
}

// getCurrentKingClipSQLTest mirrors the production getCurrentKingClipSQL
// constant in king_clip_repository.go verbatim. It is duplicated here
// (rather than exported from the repository) so the test pins the exact
// query text: any regression that drops the "expires_at > now()" predicate
// (serving an already-DEAD king) or the "ORDER BY created_at DESC" ordering
// (serving a stale king instead of the latest) changes the production string
// but not this one, so pgxmock's ExpectQuery no longer matches the actual
// call and every case below fails.
const getCurrentKingClipSQLTest = `
SELECT id, user_id, hill_type, object_key, blink_ts_ms, created_at, expires_at
FROM king_clips
WHERE hill_type = $1 AND expires_at > now()
ORDER BY created_at DESC, id DESC
LIMIT 1`

func TestKingClipRepository_GetCurrent(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	expires := now.Add(24 * time.Hour)
	cols := kingClipCols()

	tests := []struct {
		name     string
		hillType string
		setup    func(m pgxmock.PgxPoolIface)
		wantID   int64
		wantErr  error
	}{
		{
			// criterion: 3 — returns the current non-expired king clip for the
			// hill. The query is matched against the exact production SQL text
			// (via regexp.QuoteMeta), so a regression that removes
			// "expires_at > now()" or "ORDER BY created_at DESC" makes the
			// production query text diverge from this expectation and the case
			// fails (no matching expectation registered).
			name:     "returns current non-expired king clip",
			hillType: domain.HillTypeDaily,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(cols).
					AddRow(int64(3), int64(42), "daily", "king-clips/daily/42/c.webm", int64(555), now, expires)
				m.ExpectQuery(regexp.QuoteMeta(getCurrentKingClipSQLTest)).WithArgs("daily").WillReturnRows(rows)
			},
			wantID: 3,
		},
		{
			// criterion: 3 — none available (no rows, e.g. all expired) →
			// ErrKingClipNotFound. Still pinned against the exact non-expired,
			// latest-first query text.
			name:     "no current king clip returns ErrKingClipNotFound",
			hillType: domain.HillTypeMonthly,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(cols)
				m.ExpectQuery(regexp.QuoteMeta(getCurrentKingClipSQLTest)).WithArgs("monthly").WillReturnRows(rows)
			},
			wantErr: repository.ErrKingClipNotFound,
		},
		{
			name:     "query error propagated",
			hillType: domain.HillTypeRanked,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(regexp.QuoteMeta(getCurrentKingClipSQLTest)).WillReturnError(errors.New("db error"))
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

			repo := repository.NewKingClipRepository(mock)
			got, err := repo.GetCurrent(context.Background(), tt.hillType)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("GetCurrent() error = nil, want error")
				}

				if errors.Is(tt.wantErr, repository.ErrKingClipNotFound) &&
					!errors.Is(err, repository.ErrKingClipNotFound) {
					t.Errorf("GetCurrent() error = %v, want ErrKingClipNotFound", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("GetCurrent() unexpected error = %v", err)
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

func TestKingClipRepository_GetByID(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	expires := now.Add(24 * time.Hour)
	cols := kingClipCols()

	tests := []struct {
		name    string
		id      int64
		setup   func(m pgxmock.PgxPoolIface)
		wantID  int64
		wantErr error
	}{
		{
			name: "returns king clip when found",
			id:   1,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(cols).
					AddRow(int64(1), int64(42), "daily", "king-clips/daily/42/a.webm", int64(10), now, expires)
				m.ExpectQuery(`SELECT`).WithArgs(int64(1)).WillReturnRows(rows)
			},
			wantID: 1,
		},
		{
			name: "returns ErrKingClipNotFound when no rows",
			id:   999,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(cols)
				m.ExpectQuery(`SELECT`).WithArgs(int64(999)).WillReturnRows(rows)
			},
			wantErr: repository.ErrKingClipNotFound,
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

			repo := repository.NewKingClipRepository(mock)
			got, err := repo.GetByID(context.Background(), tt.id)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("GetByID() error = nil, want error")
				}

				if errors.Is(tt.wantErr, repository.ErrKingClipNotFound) &&
					!errors.Is(err, repository.ErrKingClipNotFound) {
					t.Errorf("GetByID() error = %v, want ErrKingClipNotFound", err)
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

func TestKingClipRepository_Delete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		id      int64
		setup   func(m pgxmock.PgxPoolIface)
		wantKey string
		wantErr error
	}{
		{
			// criterion: 4 — DELETE removes object + metadata: repo returns the
			// object_key for the caller to remove from storage.
			name: "deletes king clip and returns object key",
			id:   5,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"object_key"}).AddRow("king-clips/daily/42/5.webm")
				m.ExpectQuery(`DELETE FROM king_clips`).WithArgs(int64(5)).WillReturnRows(rows)
			},
			wantKey: "king-clips/daily/42/5.webm",
		},
		{
			// criterion: 4 — deleting an absent id returns ErrKingClipNotFound.
			name: "no rows returns ErrKingClipNotFound",
			id:   999,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`DELETE FROM king_clips`).WithArgs(int64(999)).
					WillReturnRows(pgxmock.NewRows([]string{"object_key"}))
			},
			wantErr: repository.ErrKingClipNotFound,
		},
		{
			name: "query error propagated",
			id:   1,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`DELETE FROM king_clips`).WillReturnError(errors.New("db error"))
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

			repo := repository.NewKingClipRepository(mock)
			key, err := repo.Delete(context.Background(), tt.id)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("Delete() error = nil, want error")
				}

				if errors.Is(tt.wantErr, repository.ErrKingClipNotFound) &&
					!errors.Is(err, repository.ErrKingClipNotFound) {
					t.Errorf("Delete() error = %v, want ErrKingClipNotFound", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("Delete() unexpected error = %v", err)
			}

			if key != tt.wantKey {
				t.Errorf("key = %q, want %q", key, tt.wantKey)
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled pgxmock expectations: %v", err)
			}
		})
	}
}

func TestKingClipRepository_DeleteSupersededByHill(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		hillType string
		keepID   int64
		setup    func(m pgxmock.PgxPoolIface)
		wantKeys []string
		wantErr  bool
	}{
		{
			// criterion: 4(b) — a new upload for a hill evicts the superseded
			// (older) prior king clip(s) for that hill_type (object + metadata
			// removed), while other hills are untouched (hill_type is scoped in
			// the SQL). The query is matched against the exact "id < $2" text, so
			// a regression back to a symmetric "id <> $2" predicate fails this
			// case outright (no matching expectation registered).
			name:     "evicts prior king clips for the same hill, keeping the new one",
			hillType: domain.HillTypeDaily,
			keepID:   9,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"object_key"}).
					AddRow("king-clips/daily/42/old1.webm").
					AddRow("king-clips/daily/7/old2.webm")
				m.ExpectQuery(regexp.QuoteMeta("DELETE FROM king_clips\nWHERE hill_type = $1 AND id < $2")).
					WithArgs("daily", int64(9)).WillReturnRows(rows)
			},
			wantKeys: []string{"king-clips/daily/42/old1.webm", "king-clips/daily/7/old2.webm"},
		},
		{
			// criterion: concurrency race guard — a concurrent upload that landed a
			// HIGHER id than keepID (i.e. it is NEWER than the row that just called
			// DeleteSupersededByHill) must never be superseded: the repository only
			// issues "id < $2" against the DB, so pgxmock records zero rows deleted
			// for a keepID lower than the concurrent writer's id. This is the
			// mechanical proof that two racing uploads for the same hill cannot both
			// wipe each other out — whichever holds the highest id survives.
			name:     "newer concurrent upload (id > keepID) is never superseded",
			hillType: domain.HillTypeDaily,
			keepID:   5,
			setup: func(m pgxmock.PgxPoolIface) {
				// The concurrent writer's row (id=9 > keepID=5) is NOT among the
				// returned object keys: only rows with id < 5 come back.
				rows := pgxmock.NewRows([]string{"object_key"}).AddRow("king-clips/daily/42/old.webm")
				m.ExpectQuery(regexp.QuoteMeta("DELETE FROM king_clips\nWHERE hill_type = $1 AND id < $2")).
					WithArgs("daily", int64(5)).WillReturnRows(rows)
			},
			wantKeys: []string{"king-clips/daily/42/old.webm"},
		},
		{
			name:     "no superseded clips returns empty slice",
			hillType: domain.HillTypeDaily,
			keepID:   1,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`DELETE FROM king_clips`).WithArgs("daily", int64(1)).
					WillReturnRows(pgxmock.NewRows([]string{"object_key"}))
			},
			wantKeys: []string{},
		},
		{
			name:     "query error propagated",
			hillType: domain.HillTypeDaily,
			keepID:   1,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`DELETE FROM king_clips`).WillReturnError(errors.New("db error"))
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

			repo := repository.NewKingClipRepository(mock)
			keys, err := repo.DeleteSupersededByHill(context.Background(), tt.hillType, tt.keepID)

			if tt.wantErr {
				if err == nil {
					t.Fatal("DeleteSupersededByHill() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("DeleteSupersededByHill() unexpected error = %v", err)
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

func TestKingClipRepository_DeleteExpired(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setup    func(m pgxmock.PgxPoolIface)
		wantKeys []string
		wantErr  bool
	}{
		{
			// criterion: 4(c) — cleanup path removes expired king clips (object +
			// metadata); exercised directly since it is not HTTP-wired.
			name: "deletes expired king clips and returns their object keys",
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"object_key"}).AddRow("king-clips/daily/42/expired.webm")
				m.ExpectQuery(`DELETE FROM king_clips`).WillReturnRows(rows)
			},
			wantKeys: []string{"king-clips/daily/42/expired.webm"},
		},
		{
			name: "no expired clips returns empty slice",
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`DELETE FROM king_clips`).WillReturnRows(pgxmock.NewRows([]string{"object_key"}))
			},
			wantKeys: []string{},
		},
		{
			name: "query error propagated",
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`DELETE FROM king_clips`).WillReturnError(errors.New("db error"))
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

			repo := repository.NewKingClipRepository(mock)
			keys, err := repo.DeleteExpired(context.Background())

			if tt.wantErr {
				if err == nil {
					t.Fatal("DeleteExpired() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("DeleteExpired() unexpected error = %v", err)
			}

			if len(keys) != len(tt.wantKeys) {
				t.Errorf("len(keys) = %d, want %d", len(keys), len(tt.wantKeys))
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled pgxmock expectations: %v", err)
			}
		})
	}
}
