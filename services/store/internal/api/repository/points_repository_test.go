package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/pizdagladki/full/services/store/internal/api/repository"
)

func TestPointsRepository_Credit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		userID      int64
		delta       int64
		reason      string
		refID       string
		setup       func(m pgxmock.PgxPoolIface)
		wantBalance int64
		wantCredit  bool
		wantErr     bool
	}{
		{
			// criterion: 1 — Credit appends a ledger row and increments the balance
			// (materializing it at 0 for a first-time user) in a SINGLE transaction.
			name:   "first time user materializes balance at 0 then credits",
			userID: 1,
			delta:  10,
			reason: "match_win",
			refID:  "match-1",
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectBegin()
				ledgerRows := pgxmock.NewRows([]string{"id"}).AddRow(int64(1))
				m.ExpectQuery(`INSERT INTO points_ledger`).
					WithArgs(int64(1), int64(10), "match_win", "match-1").
					WillReturnRows(ledgerRows)
				balanceRows := pgxmock.NewRows([]string{"balance"}).AddRow(int64(10))
				m.ExpectQuery(`INSERT INTO points_balance`).
					WithArgs(int64(1), int64(10)).
					WillReturnRows(balanceRows)
				m.ExpectCommit()
			},
			wantBalance: 10,
			wantCredit:  true,
		},
		{
			// criterion: 1 — Credit increments an EXISTING balance (not just materializes).
			name:   "existing balance increments",
			userID: 2,
			delta:  25,
			reason: "level_up",
			refID:  "lvl-2",
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectBegin()
				ledgerRows := pgxmock.NewRows([]string{"id"}).AddRow(int64(2))
				m.ExpectQuery(`INSERT INTO points_ledger`).
					WithArgs(int64(2), int64(25), "level_up", "lvl-2").
					WillReturnRows(ledgerRows)
				balanceRows := pgxmock.NewRows([]string{"balance"}).AddRow(int64(135))
				m.ExpectQuery(`INSERT INTO points_balance`).
					WithArgs(int64(2), int64(25)).
					WillReturnRows(balanceRows)
				m.ExpectCommit()
			},
			wantBalance: 135,
			wantCredit:  true,
		},
		{
			// criterion: 2 — Credit is idempotent by (user_id, reason, ref_id): a
			// duplicate reference does NOT append a second row or double-count, and
			// returns the EXISTING balance with credited=false.
			name:   "duplicate ref is idempotent returns existing balance unchanged",
			userID: 3,
			delta:  10,
			reason: "match_win",
			refID:  "match-dup",
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectBegin()
				// ON CONFLICT DO NOTHING -> no row returned -> pgx.ErrNoRows.
				m.ExpectQuery(`INSERT INTO points_ledger`).
					WithArgs(int64(3), int64(10), "match_win", "match-dup").
					WillReturnError(pgx.ErrNoRows)
				existingRows := pgxmock.NewRows([]string{"balance"}).AddRow(int64(50))
				m.ExpectQuery(`SELECT balance FROM points_balance`).
					WithArgs(int64(3)).
					WillReturnRows(existingRows)
				m.ExpectCommit()
				// NOTE: no ExpectExec/Query for points_balance INSERT — must NOT be called.
			},
			wantBalance: 50,
			wantCredit:  false,
		},
		{
			// criterion: 2 — duplicate credit for a user with no existing balance row
			// returns 0, not an error (materialize-at-0 semantics on the read side too).
			name:   "duplicate ref for user with no balance row returns 0",
			userID: 4,
			delta:  10,
			reason: "match_win",
			refID:  "match-dup2",
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectBegin()
				m.ExpectQuery(`INSERT INTO points_ledger`).
					WithArgs(int64(4), int64(10), "match_win", "match-dup2").
					WillReturnError(pgx.ErrNoRows)
				m.ExpectQuery(`SELECT balance FROM points_balance`).
					WithArgs(int64(4)).
					WillReturnError(pgx.ErrNoRows)
				m.ExpectCommit()
			},
			wantBalance: 0,
			wantCredit:  false,
		},
		{
			// criterion: 4 — an empty ref_id is converted to SQL NULL (passed as nil),
			// not the empty string, since the partial unique index only covers non-null ref_id.
			name:   "empty ref_id passed as nil not empty string",
			userID: 5,
			delta:  10,
			reason: "match_win",
			refID:  "",
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectBegin()
				ledgerRows := pgxmock.NewRows([]string{"id"}).AddRow(int64(5))
				m.ExpectQuery(`INSERT INTO points_ledger`).
					WithArgs(int64(5), int64(10), "match_win", nil).
					WillReturnRows(ledgerRows)
				balanceRows := pgxmock.NewRows([]string{"balance"}).AddRow(int64(10))
				m.ExpectQuery(`INSERT INTO points_balance`).
					WithArgs(int64(5), int64(10)).
					WillReturnRows(balanceRows)
				m.ExpectCommit()
			},
			wantBalance: 10,
			wantCredit:  true,
		},
		{
			name:   "begin error propagates",
			userID: 6,
			delta:  10,
			reason: "match_win",
			refID:  "x",
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectBegin().WillReturnError(errors.New("begin failed"))
			},
			wantErr: true,
		},
		{
			name:   "ledger insert non-duplicate error triggers rollback",
			userID: 7,
			delta:  10,
			reason: "match_win",
			refID:  "x",
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectBegin()
				m.ExpectQuery(`INSERT INTO points_ledger`).
					WithArgs(int64(7), int64(10), "match_win", "x").
					WillReturnError(errors.New("db error"))
				m.ExpectRollback()
			},
			wantErr: true,
		},
		{
			name:   "balance upsert error triggers rollback",
			userID: 8,
			delta:  10,
			reason: "match_win",
			refID:  "x",
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectBegin()
				ledgerRows := pgxmock.NewRows([]string{"id"}).AddRow(int64(9))
				m.ExpectQuery(`INSERT INTO points_ledger`).
					WithArgs(int64(8), int64(10), "match_win", "x").
					WillReturnRows(ledgerRows)
				m.ExpectQuery(`INSERT INTO points_balance`).
					WithArgs(int64(8), int64(10)).
					WillReturnError(errors.New("db error"))
				m.ExpectRollback()
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock := newPool(t)
			tt.setup(mock)

			repo := repository.NewPointsRepository(mock)
			balance, credited, err := repo.Credit(context.Background(), tt.userID, tt.delta, tt.reason, tt.refID)

			if tt.wantErr {
				if err == nil {
					t.Fatal("Credit() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("Credit() unexpected error = %v", err)
			}

			if balance != tt.wantBalance {
				t.Errorf("Credit() balance = %d, want %d", balance, tt.wantBalance)
			}

			if credited != tt.wantCredit {
				t.Errorf("Credit() credited = %v, want %v", credited, tt.wantCredit)
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestPointsRepository_GetBalance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		userID  int64
		setup   func(m pgxmock.PgxPoolIface)
		want    int64
		wantErr bool
	}{
		{
			name:   "existing balance returned",
			userID: 1,
			setup: func(m pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{"balance"}).AddRow(int64(77))
				m.ExpectQuery(`SELECT balance FROM points_balance`).WithArgs(int64(1)).WillReturnRows(rows)
			},
			want: 77,
		},
		{
			// criterion: 1 — GetBalance materializes a missing row as 0, not an error.
			name:   "missing row returns 0 not error",
			userID: 2,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`SELECT balance FROM points_balance`).WithArgs(int64(2)).WillReturnError(pgx.ErrNoRows)
			},
			want: 0,
		},
		{
			name:   "query error propagated",
			userID: 3,
			setup: func(m pgxmock.PgxPoolIface) {
				m.ExpectQuery(`SELECT balance FROM points_balance`).WithArgs(int64(3)).WillReturnError(errors.New("db error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock := newPool(t)
			tt.setup(mock)

			repo := repository.NewPointsRepository(mock)
			got, err := repo.GetBalance(context.Background(), tt.userID)

			if tt.wantErr {
				if err == nil {
					t.Fatal("GetBalance() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("GetBalance() unexpected error = %v", err)
			}

			if got != tt.want {
				t.Errorf("GetBalance() = %d, want %d", got, tt.want)
			}

			if err = mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}
