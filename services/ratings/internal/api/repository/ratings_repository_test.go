package repository

import (
	"context"
	"errors"
	"testing"

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
			input: domain.MatchInput{
				WinnerID:   1,
				LoserID:    2,
				Mode:       "classic",
				DurationMS: &dur,
			},
			setup: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectBegin()

				// Upsert winner default
				mock.ExpectExec(`INSERT INTO ratings`).
					WithArgs(int64(1), domain.DefaultELO, domain.DefaultLevel, domain.DefaultGamesPlayed).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))

				// Upsert loser default
				mock.ExpectExec(`INSERT INTO ratings`).
					WithArgs(int64(2), domain.DefaultELO, domain.DefaultLevel, domain.DefaultGamesPlayed).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))

				// Lock winner: ELO=1000, level=4, games_played=0 (calibration → K=64)
				mock.ExpectQuery(`SELECT elo, level, games_played`).
					WithArgs(int64(1)).
					WillReturnRows(mock.NewRows([]string{"elo", "level", "games_played"}).
						AddRow(1000, 4, 0))

				// Lock loser: ELO=1000, level=4, games_played=0
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

				// Insert match result
				mock.ExpectExec(`INSERT INTO match_results`).
					WithArgs(int64(1), int64(2), "classic", 32, -26, &dur).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))

				mock.ExpectCommit()
			},
			want: domain.MatchResult{
				Winner:      domain.Rating{UserID: 1, ELO: 1032, Level: 4, GamesPlayed: 1},
				Loser:       domain.Rating{UserID: 2, ELO: 974, Level: 4, GamesPlayed: 1},
				WinnerDelta: 32,
				LoserDelta:  -26,
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

				mock.ExpectExec(`INSERT INTO match_results`).
					WithArgs(int64(1), int64(2), "classic", 16, -13, (*int)(nil)).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))

				mock.ExpectCommit().WillReturnError(errors.New("write conflict"))
			},
			wantErr: true,
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
