package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/pizdagladki/full/services/koth/internal/api/domain"
	"github.com/pizdagladki/full/services/koth/internal/api/repository"
)

func newHillPool(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool() error = %v", err)
	}

	t.Cleanup(mock.Close)

	return mock
}

var kingCols = []string{"id", "user_id", "clip_id", "blink_ts_ms", "started_at"}

// TestHillRepository_CurrentKing verifies criterion: 1 — CurrentKing returns
// the current reign row when it exists, and ErrHillNotFound when the hill
// needs seeding (no current reign).
func TestHillRepository_CurrentKing(t *testing.T) {
	t.Parallel()

	started := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		hillType  domain.HillType
		setup     func(m pgxmock.PgxPoolIface)
		wantKing  *domain.KingReign
		wantErr   bool
		wantErrIs error
	}{
		{
			// criterion: 1 — a seeded hill returns the current king row
			name:     "found returns current king",
			hillType: domain.HillTypeDaily,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(kingCols).AddRow(int64(1), int64(42), "clip-1", 8000, started)
				m.ExpectQuery(`SELECT`).WithArgs("daily").WillReturnRows(rows)
			},
			wantKing: &domain.KingReign{ID: 1, HillType: domain.HillTypeDaily, UserID: 42, ClipID: "clip-1", BlinkTsMs: 8000, StartedAt: started},
		},
		{
			// criterion: 1 — no current reign for the hill_type returns ErrHillNotFound (needs seeding)
			name:     "not seeded returns ErrHillNotFound",
			hillType: domain.HillTypeMonthly,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`SELECT`).WithArgs("monthly").WillReturnError(pgx.ErrNoRows)
			},
			wantErr:   true,
			wantErrIs: repository.ErrHillNotFound,
		},
		{
			name:     "query error propagated",
			hillType: domain.HillTypeDaily,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`SELECT`).WithArgs("daily").WillReturnError(errors.New("db error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock := newHillPool(t)
			tt.setup(mock)

			repo := repository.NewHillRepository(mock)
			got, err := repo.CurrentKing(context.Background(), tt.hillType)

			if tt.wantErr {
				if err == nil {
					t.Fatal("CurrentKing() error = nil, want error")
				}

				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Errorf("CurrentKing() error = %v, want %v", err, tt.wantErrIs)
				}

				return
			}

			if err != nil {
				t.Fatalf("CurrentKing() unexpected error = %v", err)
			}

			if *got != *tt.wantKing {
				t.Errorf("CurrentKing() = %+v, want %+v", got, tt.wantKing)
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

// TestHillRepository_Challenge verifies criteria 2 and 3 — Challenge closes
// the current reign and opens a new one for the challenger when
// survived_ms >= the king's blink_ts_ms (won), leaves the reign untouched
// otherwise (lost), and returns ErrHillNotFound when the hill isn't seeded.
func TestHillRepository_Challenge(t *testing.T) {
	t.Parallel()

	newStarted := time.Date(2026, 7, 3, 15, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		hillType   domain.HillType
		userID     int64
		survivedMs int
		newClipID  string
		setup      func(m pgxmock.PgxPoolIface)
		wantWon    bool
		wantKing   domain.KingReign
		wantErr    bool
		wantErrIs  error
	}{
		{
			// criterion: 2 — a challenger who beats the king's blink_ts_ms closes the
			// old reign and opens a new one with their own clip + blink_ts_ms
			name:       "challenger wins takes crown",
			hillType:   domain.HillTypeDaily,
			userID:     99,
			survivedMs: 9000,
			newClipID:  "clip-new",
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectBegin()
				m.ExpectExec(`pg_advisory_xact_lock`).WithArgs("daily").
					WillReturnResult(pgxmock.NewResult("SELECT", 0))
				m.ExpectQuery(`SELECT`).WithArgs("daily").
					WillReturnRows(pgxmock.NewRows(kingCols).AddRow(int64(1), int64(42), "clip-1", 8000, time.Now()))
				m.ExpectExec(`UPDATE king_reigns`).WithArgs(int64(1)).
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))
				m.ExpectQuery(`INSERT INTO king_reigns`).
					WithArgs("daily", int64(99), "clip-new", 9000).
					WillReturnRows(pgxmock.NewRows([]string{"id", "started_at"}).AddRow(int64(2), newStarted))
				m.ExpectCommit()
			},
			wantWon: true,
			wantKing: domain.KingReign{
				ID: 2, HillType: domain.HillTypeDaily, UserID: 99, ClipID: "clip-new",
				BlinkTsMs: 9000, StartedAt: newStarted,
			},
		},
		{
			// criterion: 2 — a challenger who falls short leaves the reign unchanged
			name:       "challenger loses king stays",
			hillType:   domain.HillTypeDaily,
			userID:     99,
			survivedMs: 5000,
			newClipID:  "clip-new",
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectBegin()
				m.ExpectExec(`pg_advisory_xact_lock`).WithArgs("daily").
					WillReturnResult(pgxmock.NewResult("SELECT", 0))
				m.ExpectQuery(`SELECT`).WithArgs("daily").
					WillReturnRows(pgxmock.NewRows(kingCols).AddRow(int64(1), int64(42), "clip-1", 8000, time.Now()))
				m.ExpectCommit()
			},
			wantWon: false,
			wantKing: domain.KingReign{
				ID: 1, HillType: domain.HillTypeDaily, UserID: 42, ClipID: "clip-1", BlinkTsMs: 8000,
			},
		},
		{
			// criterion: 6 — seeding-404: challenging an unseeded hill returns ErrHillNotFound
			name:       "hill not seeded returns ErrHillNotFound",
			hillType:   domain.HillTypeMonthly,
			userID:     99,
			survivedMs: 5000,
			newClipID:  "clip-new",
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectBegin()
				m.ExpectExec(`pg_advisory_xact_lock`).WithArgs("monthly").
					WillReturnResult(pgxmock.NewResult("SELECT", 0))
				m.ExpectQuery(`SELECT`).WithArgs("monthly").WillReturnError(pgx.ErrNoRows)
				m.ExpectRollback()
			},
			wantErr:   true,
			wantErrIs: repository.ErrHillNotFound,
		},
		{
			name:       "begin tx error propagated",
			hillType:   domain.HillTypeDaily,
			userID:     99,
			survivedMs: 5000,
			newClipID:  "clip-new",
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectBegin().WillReturnError(errors.New("pool exhausted"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock := newHillPool(t)
			tt.setup(mock)

			repo := repository.NewHillRepository(mock)
			got, err := repo.Challenge(context.Background(), tt.hillType, tt.userID, tt.survivedMs, tt.newClipID)

			if tt.wantErr {
				if err == nil {
					t.Fatal("Challenge() error = nil, want error")
				}

				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Errorf("Challenge() error = %v, want %v", err, tt.wantErrIs)
				}

				return
			}

			if err != nil {
				t.Fatalf("Challenge() unexpected error = %v", err)
			}

			if got.Won != tt.wantWon {
				t.Errorf("Challenge() Won = %v, want %v", got.Won, tt.wantWon)
			}

			if got.King.ID != tt.wantKing.ID || got.King.UserID != tt.wantKing.UserID ||
				got.King.ClipID != tt.wantKing.ClipID || got.King.BlinkTsMs != tt.wantKing.BlinkTsMs {
				t.Errorf("Challenge() King = %+v, want %+v", got.King, tt.wantKing)
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

// TestHillRepository_Challenge_ConcurrentSerialization verifies criterion: 4 —
// two "simultaneous" winning challenges against the same hill_type must result
// in EXACTLY ONE crown transfer: the advisory lock serializes the two calls
// (represented here by pgxmock's ordered expectation queue), and the second
// caller is re-evaluated against the FRESH king written by the first — even
// though both survived_ms values would have beaten the ORIGINAL king, only
// the first actually transfers the crown.
func TestHillRepository_Challenge_ConcurrentSerialization(t *testing.T) {
	t.Parallel()

	mock := newHillPool(t)
	t.Cleanup(mock.Close)

	// Both challengers A (survived_ms=10000) and B (survived_ms=8000) would
	// beat the original king (blink_ts_ms=3000) if compared to stale data.
	//
	// Call 1 (challenger A, userID=1): sees the original king, wins, becomes
	// king with blink_ts_ms=10000.
	mock.ExpectBegin()
	mock.ExpectExec(`pg_advisory_xact_lock`).WithArgs("daily").
		WillReturnResult(pgxmock.NewResult("SELECT", 0))
	mock.ExpectQuery(`SELECT`).WithArgs("daily").
		WillReturnRows(pgxmock.NewRows(kingCols).AddRow(int64(1), int64(7), "clip-orig", 3000, time.Now()))
	mock.ExpectExec(`UPDATE king_reigns`).WithArgs(int64(1)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectQuery(`INSERT INTO king_reigns`).
		WithArgs("daily", int64(1), "clip-a", 10000).
		WillReturnRows(pgxmock.NewRows([]string{"id", "started_at"}).AddRow(int64(2), time.Now()))
	mock.ExpectCommit()

	// Call 2 (challenger B, userID=2), serialized AFTER call 1 commits: the
	// lock forces a fresh re-read, which now returns A's reign
	// (blink_ts_ms=10000). B's 8000 < 10000, so B loses — no second transfer.
	mock.ExpectBegin()
	mock.ExpectExec(`pg_advisory_xact_lock`).WithArgs("daily").
		WillReturnResult(pgxmock.NewResult("SELECT", 0))
	mock.ExpectQuery(`SELECT`).WithArgs("daily").
		WillReturnRows(pgxmock.NewRows(kingCols).AddRow(int64(2), int64(1), "clip-a", 10000, time.Now()))
	mock.ExpectCommit()

	repo := repository.NewHillRepository(mock)

	outcomeA, err := repo.Challenge(context.Background(), domain.HillTypeDaily, 1, 10000, "clip-a")
	if err != nil {
		t.Fatalf("challenger A Challenge() unexpected error = %v", err)
	}

	if !outcomeA.Won {
		t.Fatal("challenger A: Won = false, want true (first mover should take the crown)")
	}

	outcomeB, err := repo.Challenge(context.Background(), domain.HillTypeDaily, 2, 8000, "clip-b")
	if err != nil {
		t.Fatalf("challenger B Challenge() unexpected error = %v", err)
	}

	if outcomeB.Won {
		t.Fatal("challenger B: Won = true, want false — exactly one crown transfer must occur, " +
			"and B must be re-evaluated against A's fresh reign, not the stale original king")
	}

	if outcomeB.King.UserID != 1 || outcomeB.King.BlinkTsMs != 10000 {
		t.Errorf("challenger B: King = %+v, want the current king to still be A (user_id=1, blink_ts_ms=10000)",
			outcomeB.King)
	}

	if err = mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}
