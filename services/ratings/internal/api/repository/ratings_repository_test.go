package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/pizdagladki/full/services/ratings/internal/api/domain"
)

// newMock creates a pgxmock pool (ordered) and a repository wired to it.
func newMock(t *testing.T) (pgxmock.PgxPoolIface, RatingsRepository) {
	t.Helper()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}

	repo := NewRatingsRepositoryFromDB(mock)

	return mock, repo
}

// ─── ListMatchHistory ─────────────────────────────────────────────────────────

func TestListMatchHistory(t *testing.T) {
	t.Parallel()

	dur := 5000
	now := time.Now().UTC().Truncate(time.Second)
	earlier := now.Add(-time.Hour)

	tests := []struct {
		name    string
		userID  int64
		limit   int
		offset  int
		setup   func(mock pgxmock.PgxPoolIface)
		want    []domain.MatchHistoryItem
		wantErr bool
	}{
		{
			// criterion: 1 — returns matches where user is winner OR loser, newest first
			// criterion: 2 — result and elo_delta derived correctly for winner perspective
			name:   "winner perspective: result=win, own elo_delta=winner_elo_delta",
			userID: 1,
			limit:  10,
			offset: 0,
			setup: func(mock pgxmock.PgxPoolIface) {
				rows := mock.NewRows([]string{"id", "opponent_id", "result", "mode", "elo_delta", "duration_ms", "created_at"}).
					AddRow(int64(42), int64(2), "win", "classic", 32, &dur, now)
				mock.ExpectQuery(`SELECT`).
					WithArgs(int64(1), 10, 0).
					WillReturnRows(rows)
			},
			want: []domain.MatchHistoryItem{
				{MatchID: 42, OpponentID: 2, Result: "win", Mode: "classic", ELODelta: 32, DurationMS: &dur, CreatedAt: now},
			},
		},
		{
			// criterion: 2 — result and elo_delta derived correctly for loser perspective
			name:   "loser perspective: result=loss, own elo_delta=loser_elo_delta",
			userID: 2,
			limit:  10,
			offset: 0,
			setup: func(mock pgxmock.PgxPoolIface) {
				rows := mock.NewRows([]string{"id", "opponent_id", "result", "mode", "elo_delta", "duration_ms", "created_at"}).
					AddRow(int64(42), int64(1), "loss", "classic", -26, &dur, now)
				mock.ExpectQuery(`SELECT`).
					WithArgs(int64(2), 10, 0).
					WillReturnRows(rows)
			},
			want: []domain.MatchHistoryItem{
				{MatchID: 42, OpponentID: 1, Result: "loss", Mode: "classic", ELODelta: -26, DurationMS: &dur, CreatedAt: now},
			},
		},
		{
			// criterion: 1 — newest first ordering (two rows, descending created_at)
			name:   "newest-first ordering: two rows returned in DESC created_at order",
			userID: 1,
			limit:  10,
			offset: 0,
			setup: func(mock pgxmock.PgxPoolIface) {
				rows := mock.NewRows([]string{"id", "opponent_id", "result", "mode", "elo_delta", "duration_ms", "created_at"}).
					AddRow(int64(100), int64(3), "win", "ranked", 16, nil, now).
					AddRow(int64(99), int64(2), "loss", "classic", -13, &dur, earlier)
				mock.ExpectQuery(`SELECT`).
					WithArgs(int64(1), 10, 0).
					WillReturnRows(rows)
			},
			want: []domain.MatchHistoryItem{
				{MatchID: 100, OpponentID: 3, Result: "win", Mode: "ranked", ELODelta: 16, DurationMS: nil, CreatedAt: now},
				{MatchID: 99, OpponentID: 2, Result: "loss", Mode: "classic", ELODelta: -13, DurationMS: &dur, CreatedAt: earlier},
			},
		},
		{
			// criterion: 3 — pagination: limit/offset passed through to query
			name:   "pagination: limit and offset passed to query",
			userID: 1,
			limit:  2,
			offset: 5,
			setup: func(mock pgxmock.PgxPoolIface) {
				rows := mock.NewRows([]string{"id", "opponent_id", "result", "mode", "elo_delta", "duration_ms", "created_at"}).
					AddRow(int64(10), int64(9), "win", "classic", 20, nil, now)
				mock.ExpectQuery(`SELECT`).
					WithArgs(int64(1), 2, 5).
					WillReturnRows(rows)
			},
			want: []domain.MatchHistoryItem{
				{MatchID: 10, OpponentID: 9, Result: "win", Mode: "classic", ELODelta: 20, DurationMS: nil, CreatedAt: now},
			},
		},
		{
			// criterion: 4 — user with no matches returns empty slice, not nil
			name:   "empty result returns empty slice not nil",
			userID: 99,
			limit:  20,
			offset: 0,
			setup: func(mock pgxmock.PgxPoolIface) {
				rows := mock.NewRows([]string{"id", "opponent_id", "result", "mode", "elo_delta", "duration_ms", "created_at"})
				mock.ExpectQuery(`SELECT`).
					WithArgs(int64(99), 20, 0).
					WillReturnRows(rows)
			},
			want: []domain.MatchHistoryItem{},
		},
		{
			name:   "db query error is propagated",
			userID: 1,
			limit:  10,
			offset: 0,
			setup: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery(`SELECT`).
					WithArgs(int64(1), 10, 0).
					WillReturnError(errors.New("connection reset"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock, repo := newMock(t)
			tt.setup(mock)

			got, err := repo.ListMatchHistory(context.Background(), tt.userID, tt.limit, tt.offset)

			if tt.wantErr {
				if err == nil {
					t.Fatal("ListMatchHistory() error = nil, want error")
				}

				if metErr := mock.ExpectationsWereMet(); metErr != nil {
					t.Errorf("unmet expectations on error path: %v", metErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("ListMatchHistory() unexpected error = %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("ListMatchHistory() len = %d, want %d; got %+v", len(got), len(tt.want), got)
			}

			for i, item := range got {
				w := tt.want[i]
				if item.MatchID != w.MatchID {
					t.Errorf("[%d] MatchID = %d, want %d", i, item.MatchID, w.MatchID)
				}

				if item.OpponentID != w.OpponentID {
					t.Errorf("[%d] OpponentID = %d, want %d", i, item.OpponentID, w.OpponentID)
				}

				if item.Result != w.Result {
					t.Errorf("[%d] Result = %q, want %q", i, item.Result, w.Result)
				}

				if item.ELODelta != w.ELODelta {
					t.Errorf("[%d] ELODelta = %d, want %d", i, item.ELODelta, w.ELODelta)
				}

				if item.Mode != w.Mode {
					t.Errorf("[%d] Mode = %q, want %q", i, item.Mode, w.Mode)
				}

				if !item.CreatedAt.Equal(w.CreatedAt) {
					t.Errorf("[%d] CreatedAt = %v, want %v", i, item.CreatedAt, w.CreatedAt)
				}
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unmet expectations: %v", err)
			}
		})
	}
}

// ─── GetRating ────────────────────────────────────────────────────────────────

func TestGetRating(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(mock pgxmock.PgxPoolIface)
		userID  int64
		want    domain.Rating
		wantErr bool
	}{
		{
			name: "existing player returns stored values",
			setup: func(mock pgxmock.PgxPoolIface) {
				rows := mock.NewRows([]string{"elo", "level", "games_played"}).
					AddRow(1200, 5, 30)
				mock.ExpectQuery(`SELECT elo, level, games_played`).
					WithArgs(int64(1)).
					WillReturnRows(rows)
			},
			userID: 1,
			want:   domain.Rating{UserID: 1, ELO: 1200, Level: 5, GamesPlayed: 30},
		},
		{
			name: "unknown player returns defaults without error",
			setup: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery(`SELECT elo, level, games_played`).
					WithArgs(int64(99)).
					WillReturnError(pgx.ErrNoRows)
			},
			userID: 99,
			want: domain.Rating{
				UserID:      99,
				ELO:         domain.DefaultELO,
				Level:       domain.DefaultLevel,
				GamesPlayed: domain.DefaultGamesPlayed,
			},
		},
		{
			name: "db error is propagated",
			setup: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery(`SELECT elo, level, games_played`).
					WithArgs(int64(7)).
					WillReturnError(errors.New("connection reset"))
			},
			userID:  7,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock, repo := newMock(t)
			tt.setup(mock)

			got, err := repo.GetRating(context.Background(), tt.userID)

			if tt.wantErr {
				if err == nil {
					t.Fatal("GetRating() error = nil, want error")
				}

				if metErr := mock.ExpectationsWereMet(); metErr != nil {
					t.Errorf("unmet expectations on error path: %v", metErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("GetRating() unexpected error = %v", err)
			}

			if got != tt.want {
				t.Errorf("GetRating() = %+v, want %+v", got, tt.want)
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unmet expectations: %v", err)
			}
		})
	}
}

// ─── ApplyMatchResult ─────────────────────────────────────────────────────────

func TestApplyMatchResult(t *testing.T) {
	t.Parallel()

	dur := 5000

	tests := []struct {
		name    string
		input   domain.MatchInput
		setup   func(mock pgxmock.PgxPoolIface)
		want    domain.MatchResult
		wantErr bool
	}{
		{
			name: "happy path: two fresh players at default ELO",
			// winner=1 < loser=2 → canonical order is 1,2 (matches winner/loser order).
			input: domain.MatchInput{
				WinnerID:   1,
				LoserID:    2,
				Mode:       "classic",
				DurationMS: &dur,
			},
			setup: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectBegin()

				// Upsert in canonical (ascending-id) order: id=1 first, id=2 second.
				mock.ExpectExec(`INSERT INTO ratings`).
					WithArgs(int64(1), domain.DefaultELO, domain.DefaultLevel, domain.DefaultGamesPlayed).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))
				mock.ExpectExec(`INSERT INTO ratings`).
					WithArgs(int64(2), domain.DefaultELO, domain.DefaultLevel, domain.DefaultGamesPlayed).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))

				// Lock in canonical (ascending-id) order: id=1 first, id=2 second.
				// winner=1 ELO=1000, level=4, games_played=0 (calibration → K=64)
				mock.ExpectQuery(`SELECT elo, level, games_played`).
					WithArgs(int64(1)).
					WillReturnRows(mock.NewRows([]string{"elo", "level", "games_played"}).
						AddRow(1000, 4, 0))
				// loser=2 ELO=1000, level=4, games_played=0
				mock.ExpectQuery(`SELECT elo, level, games_played`).
					WithArgs(int64(2)).
					WillReturnRows(mock.NewRows([]string{"elo", "level", "games_played"}).
						AddRow(1000, 4, 0))

				// Equal new-player ELOs → K=64: winnerDelta=32, loserDelta=-26
				newWinnerELO := 1000 + 32   // 1032 → level 4
				newLoserELO := 1000 + (-26) // 974 → level 4

				// Update winner
				mock.ExpectExec(`UPDATE ratings`).
					WithArgs(int64(1), newWinnerELO, domain.LevelForELO(newWinnerELO), 1).
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))

				// Update loser
				mock.ExpectExec(`UPDATE ratings`).
					WithArgs(int64(2), newLoserELO, domain.LevelForELO(newLoserELO), 1).
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))

				// Insert match result — RETURNING id (criterion: MatchID populated from the row).
				mock.ExpectQuery(`INSERT INTO match_results`).
					WithArgs(int64(1), int64(2), "classic", 32, -26, &dur).
					WillReturnRows(mock.NewRows([]string{"id"}).AddRow(int64(501)))

				mock.ExpectCommit()
			},
			// criterion: 1 — MatchID is the id returned by the INSERT.
			// criterion: 2 (no band change) — winner stays at level 4 → WinnerLeveledUp=false.
			want: domain.MatchResult{
				Winner:          domain.Rating{UserID: 1, ELO: 1032, Level: 4, GamesPlayed: 1},
				Loser:           domain.Rating{UserID: 2, ELO: 974, Level: 4, GamesPlayed: 1},
				WinnerDelta:     32,
				LoserDelta:      -26,
				MatchID:         501,
				WinnerLeveledUp: false,
			},
		},
		{
			name: "canonical lock order: winner_id > loser_id locks lower id first",
			// winner=5, loser=3 → canonical order must be 3 first, then 5 (ascending).
			// This verifies the deadlock-prevention reordering: even though A is the
			// loser, its row is locked first because 3 < 5.
			input: domain.MatchInput{
				WinnerID:   5,
				LoserID:    3,
				Mode:       "ranked",
				DurationMS: &dur,
			},
			setup: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectBegin()

				// Canonical upsert order: lower id=3 first, then id=5.
				mock.ExpectExec(`INSERT INTO ratings`).
					WithArgs(int64(3), domain.DefaultELO, domain.DefaultLevel, domain.DefaultGamesPlayed).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))
				mock.ExpectExec(`INSERT INTO ratings`).
					WithArgs(int64(5), domain.DefaultELO, domain.DefaultLevel, domain.DefaultGamesPlayed).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))

				// Canonical lock order: lower id=3 first (this is the loser), then id=5 (winner).
				mock.ExpectQuery(`SELECT elo, level, games_played`).
					WithArgs(int64(3)).
					WillReturnRows(mock.NewRows([]string{"elo", "level", "games_played"}).
						AddRow(1000, 4, 0))
				mock.ExpectQuery(`SELECT elo, level, games_played`).
					WithArgs(int64(5)).
					WillReturnRows(mock.NewRows([]string{"elo", "level", "games_played"}).
						AddRow(1000, 4, 0))

				// Equal new-player ELOs → K=64: winnerDelta=32, loserDelta=-26
				newWinnerELO := 1000 + 32
				newLoserELO := 1000 + (-26)

				// Updates are applied in winner/loser order (id=5 winner, id=3 loser).
				mock.ExpectExec(`UPDATE ratings`).
					WithArgs(int64(5), newWinnerELO, domain.LevelForELO(newWinnerELO), 1).
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))
				mock.ExpectExec(`UPDATE ratings`).
					WithArgs(int64(3), newLoserELO, domain.LevelForELO(newLoserELO), 1).
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))

				mock.ExpectQuery(`INSERT INTO match_results`).
					WithArgs(int64(5), int64(3), "ranked", 32, -26, &dur).
					WillReturnRows(mock.NewRows([]string{"id"}).AddRow(int64(502)))

				mock.ExpectCommit()
			},
			want: domain.MatchResult{
				Winner:          domain.Rating{UserID: 5, ELO: 1032, Level: 4, GamesPlayed: 1},
				Loser:           domain.Rating{UserID: 3, ELO: 974, Level: 4, GamesPlayed: 1},
				WinnerDelta:     32,
				LoserDelta:      -26,
				MatchID:         502,
				WinnerLeveledUp: false,
			},
		},
		{
			name: "begin tx error",
			input: domain.MatchInput{
				WinnerID: 10,
				LoserID:  20,
				Mode:     "classic",
			},
			setup: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectBegin().WillReturnError(errors.New("pool exhausted"))
			},
			wantErr: true,
		},
		{
			name: "upsert winner fails — rollback and return error",
			input: domain.MatchInput{
				WinnerID: 10,
				LoserID:  20,
				Mode:     "classic",
			},
			setup: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectBegin()
				mock.ExpectExec(`INSERT INTO ratings`).
					WithArgs(int64(10), domain.DefaultELO, domain.DefaultLevel, domain.DefaultGamesPlayed).
					WillReturnError(errors.New("disk full"))
				mock.ExpectRollback()
			},
			wantErr: true,
		},
		{
			name: "lock winner row fails — rollback and return error",
			input: domain.MatchInput{
				WinnerID: 10,
				LoserID:  20,
				Mode:     "classic",
			},
			setup: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectBegin()
				mock.ExpectExec(`INSERT INTO ratings`).
					WithArgs(int64(10), domain.DefaultELO, domain.DefaultLevel, domain.DefaultGamesPlayed).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))
				mock.ExpectExec(`INSERT INTO ratings`).
					WithArgs(int64(20), domain.DefaultELO, domain.DefaultLevel, domain.DefaultGamesPlayed).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))
				mock.ExpectQuery(`SELECT elo, level, games_played`).
					WithArgs(int64(10)).
					WillReturnError(errors.New("deadlock"))
				mock.ExpectRollback()
			},
			wantErr: true,
		},
		{
			name: "commit fails — return error",
			input: domain.MatchInput{
				WinnerID: 1,
				LoserID:  2,
				Mode:     "classic",
			},
			setup: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectBegin()

				mock.ExpectExec(`INSERT INTO ratings`).
					WithArgs(int64(1), domain.DefaultELO, domain.DefaultLevel, domain.DefaultGamesPlayed).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))
				mock.ExpectExec(`INSERT INTO ratings`).
					WithArgs(int64(2), domain.DefaultELO, domain.DefaultLevel, domain.DefaultGamesPlayed).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))

				mock.ExpectQuery(`SELECT elo, level, games_played`).
					WithArgs(int64(1)).
					WillReturnRows(mock.NewRows([]string{"elo", "level", "games_played"}).
						AddRow(1000, 4, 20))
				mock.ExpectQuery(`SELECT elo, level, games_played`).
					WithArgs(int64(2)).
					WillReturnRows(mock.NewRows([]string{"elo", "level", "games_played"}).
						AddRow(1000, 4, 20))

				// K=32 equal → +16 / -13
				mock.ExpectExec(`UPDATE ratings`).
					WithArgs(int64(1), 1016, domain.LevelForELO(1016), 21).
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))
				mock.ExpectExec(`UPDATE ratings`).
					WithArgs(int64(2), 987, domain.LevelForELO(987), 21).
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))

				mock.ExpectQuery(`INSERT INTO match_results`).
					WithArgs(int64(1), int64(2), "classic", 16, -13, (*int)(nil)).
					WillReturnRows(mock.NewRows([]string{"id"}).AddRow(int64(999)))

				mock.ExpectCommit().WillReturnError(errors.New("write conflict"))
			},
			wantErr: true,
		},
		{
			// criterion: 2 — winner's level band strictly increases → WinnerLeveledUp=true,
			// asserted at the L4→L5 band boundary (900 → 1102 crosses 1100/1101 boundary).
			name: "winner crosses a level band boundary — WinnerLeveledUp true",
			input: domain.MatchInput{
				WinnerID: 1,
				LoserID:  2,
				Mode:     "classic",
			},
			setup: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectBegin()

				mock.ExpectExec(`INSERT INTO ratings`).
					WithArgs(int64(1), domain.DefaultELO, domain.DefaultLevel, domain.DefaultGamesPlayed).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))
				mock.ExpectExec(`INSERT INTO ratings`).
					WithArgs(int64(2), domain.DefaultELO, domain.DefaultLevel, domain.DefaultGamesPlayed).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))

				// winner starts at ELO=1080 level 4 (≤1100), games_played=20 → K=32.
				mock.ExpectQuery(`SELECT elo, level, games_played`).
					WithArgs(int64(1)).
					WillReturnRows(mock.NewRows([]string{"elo", "level", "games_played"}).
						AddRow(1080, domain.LevelForELO(1080), 20))
				// loser starts at ELO=1400 level 6, games_played=20 → K=32.
				mock.ExpectQuery(`SELECT elo, level, games_played`).
					WithArgs(int64(2)).
					WillReturnRows(mock.NewRows([]string{"elo", "level", "games_played"}).
						AddRow(1400, domain.LevelForELO(1400), 20))

				// Upset: 1080 beats 1400 (K=32) → winnerDelta=+28 → new ELO=1108 → level 5 (crosses the 1100 boundary).
				newWinnerELO := 1080 + 28
				newLoserELO := 1400 - 22

				mock.ExpectExec(`UPDATE ratings`).
					WithArgs(int64(1), newWinnerELO, domain.LevelForELO(newWinnerELO), 21).
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))
				mock.ExpectExec(`UPDATE ratings`).
					WithArgs(int64(2), newLoserELO, domain.LevelForELO(newLoserELO), 21).
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))

				mock.ExpectQuery(`INSERT INTO match_results`).
					WithArgs(int64(1), int64(2), "classic", 28, -22, (*int)(nil)).
					WillReturnRows(mock.NewRows([]string{"id"}).AddRow(int64(777)))

				mock.ExpectCommit()
			},
			want: domain.MatchResult{
				Winner:          domain.Rating{UserID: 1, ELO: 1108, Level: 5, GamesPlayed: 21},
				Loser:           domain.Rating{UserID: 2, ELO: 1378, Level: 6, GamesPlayed: 21},
				WinnerDelta:     28,
				LoserDelta:      -22,
				MatchID:         777,
				WinnerLeveledUp: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock, repo := newMock(t)
			tt.setup(mock)

			got, err := repo.ApplyMatchResult(context.Background(), tt.input)

			if tt.wantErr {
				if err == nil {
					t.Fatal("ApplyMatchResult() error = nil, want error")
				}

				if metErr := mock.ExpectationsWereMet(); metErr != nil {
					t.Errorf("unmet expectations on error path: %v", metErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("ApplyMatchResult() unexpected error = %v", err)
			}

			if got != tt.want {
				t.Errorf("ApplyMatchResult() = %+v, want %+v", got, tt.want)
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unmet expectations: %v", err)
			}
		})
	}
}
