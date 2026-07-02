package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/pizdagladki/full/services/koth/internal/api/repository"
)

func newRankPool(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool() error = %v", err)
	}

	t.Cleanup(mock.Close)

	return mock
}

var testDay = time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)

func TestRankRepository_GetRank(t *testing.T) {
	t.Parallel()

	cols := []string{"user_id", "day", "current_rank", "best_hold_ms"}

	tests := []struct {
		name      string
		userID    int64
		setup     func(m pgxmock.PgxPoolIface)
		wantRank  int
		wantErr   bool
		wantErrIs error
	}{
		{
			// criterion: 1 — GetRank returns the stored rank row when present
			name:   "found returns rank row",
			userID: 5,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(cols).AddRow(int64(5), testDay, 2, 16000)
				m.ExpectQuery(`SELECT`).WithArgs(int64(5), testDay).WillReturnRows(rows)
			},
			wantRank: 2,
		},
		{
			// criterion: 1 — GetRank returns ErrRankNotFound when no row exists for today
			name:   "not found returns ErrRankNotFound",
			userID: 9,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`SELECT`).WithArgs(int64(9), testDay).WillReturnError(pgx.ErrNoRows)
			},
			wantErr:   true,
			wantErrIs: repository.ErrRankNotFound,
		},
		{
			name:   "query error propagated",
			userID: 1,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`SELECT`).WithArgs(int64(1), testDay).WillReturnError(errors.New("db error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock := newRankPool(t)
			tt.setup(mock)

			repo := repository.NewRankRepository(mock)
			got, err := repo.GetRank(context.Background(), tt.userID, testDay)

			if tt.wantErr {
				if err == nil {
					t.Fatal("GetRank() error = nil, want error")
				}

				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Errorf("GetRank() error = %v, want %v", err, tt.wantErrIs)
				}

				return
			}

			if err != nil {
				t.Fatalf("GetRank() unexpected error = %v", err)
			}

			if got.Rank != tt.wantRank {
				t.Errorf("rank = %d, want %d", got.Rank, tt.wantRank)
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestRankRepository_UpsertRank(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		userID     int64
		newRank    int
		bestHoldMs int
		setup      func(m pgxmock.PgxPoolIface)
		wantErr    bool
	}{
		{
			// criterion: 1 — UpsertRank writes the new rank + best_hold_ms for (user_id, day)
			name:       "success upserts row",
			userID:     5,
			newRank:    2,
			bestHoldMs: 16000,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectExec(`INSERT INTO hill_ranks`).
					WithArgs(int64(5), testDay, 2, 16000).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))
			},
		},
		{
			name:       "exec error propagated",
			userID:     5,
			newRank:    2,
			bestHoldMs: 16000,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectExec(`INSERT INTO hill_ranks`).
					WithArgs(int64(5), testDay, 2, 16000).
					WillReturnError(errors.New("db error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock := newRankPool(t)
			tt.setup(mock)

			repo := repository.NewRankRepository(mock)
			err := repo.UpsertRank(context.Background(), tt.userID, testDay, tt.newRank, tt.bestHoldMs)

			if tt.wantErr {
				if err == nil {
					t.Fatal("UpsertRank() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("UpsertRank() unexpected error = %v", err)
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestRankRepository_RankDistribution(t *testing.T) {
	t.Parallel()

	cols := []string{"current_rank", "count"}

	tests := []struct {
		name    string
		setup   func(m pgxmock.PgxPoolIface)
		want    []int // ranks in order, for a coarse assertion
		wantErr bool
	}{
		{
			// criterion: 3 — RankDistribution returns accounts-per-rank counts for today, ordered by rank
			name: "returns distribution ordered by rank",
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(cols).
					AddRow(0, 3).
					AddRow(1, 5).
					AddRow(2, 1)
				m.ExpectQuery(`SELECT current_rank, COUNT`).WithArgs(testDay).WillReturnRows(rows)
			},
			want: []int{0, 1, 2},
		},
		{
			name: "empty distribution returns empty slice",
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows(cols)
				m.ExpectQuery(`SELECT current_rank, COUNT`).WithArgs(testDay).WillReturnRows(rows)
			},
			want: []int{},
		},
		{
			name: "query error propagated",
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`SELECT current_rank, COUNT`).WithArgs(testDay).WillReturnError(errors.New("db error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock := newRankPool(t)
			tt.setup(mock)

			repo := repository.NewRankRepository(mock)
			got, err := repo.RankDistribution(context.Background(), testDay)

			if tt.wantErr {
				if err == nil {
					t.Fatal("RankDistribution() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("RankDistribution() unexpected error = %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("len(distribution) = %d, want %d", len(got), len(tt.want))
			}

			for i, rc := range got {
				if rc.Rank != tt.want[i] {
					t.Errorf("distribution[%d].Rank = %d, want %d", i, rc.Rank, tt.want[i])
				}
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}
