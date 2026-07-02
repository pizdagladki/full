package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type pointsRepository struct {
	pool purchasePoolIface
}

// NewPointsRepository returns a PointsRepository backed by pool.
// In production pass *pgxpool.Pool; in tests pass pgxmock.PgxPoolIface.
func NewPointsRepository(pool purchasePoolIface) PointsRepository {
	return &pointsRepository{pool: pool}
}

const insertLedgerSQL = `
INSERT INTO points_ledger (user_id, delta, reason, ref_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id, reason, ref_id) WHERE ref_id IS NOT NULL DO NOTHING
RETURNING id`

const upsertBalanceSQL = `
INSERT INTO points_balance (user_id, balance)
VALUES ($1, $2)
ON CONFLICT (user_id) DO UPDATE
SET balance = points_balance.balance + EXCLUDED.balance, updated_at = now()
RETURNING balance`

const getBalanceSQL = `
SELECT balance FROM points_balance WHERE user_id = $1`

func (r *pointsRepository) Credit(
	ctx context.Context,
	userID, delta int64,
	reason, refID string,
) (int64, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	var refArg any
	if refID != "" {
		refArg = refID
	} // else leave refArg nil -> SQL NULL; a null-ref credit is intentionally not idempotent.

	var ledgerID int64

	err = tx.QueryRow(ctx, insertLedgerSQL, userID, delta, reason, refArg).Scan(&ledgerID)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return 0, false, fmt.Errorf("insert points ledger: %w", err)
		}

		// Duplicate reference: idempotent hit. Read the existing balance
		// within the same transaction and do not increment it.
		var balance int64

		err = tx.QueryRow(ctx, getBalanceSQL, userID).Scan(&balance)
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				return 0, false, fmt.Errorf("read balance on duplicate credit: %w", err)
			}

			balance = 0
			err = nil
		}

		err = tx.Commit(ctx)
		if err != nil {
			return 0, false, fmt.Errorf("commit duplicate credit: %w", err)
		}

		return balance, false, nil
	}

	var newBalance int64

	err = tx.QueryRow(ctx, upsertBalanceSQL, userID, delta).Scan(&newBalance)
	if err != nil {
		return 0, false, fmt.Errorf("upsert points balance: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("commit points credit: %w", err)
	}

	return newBalance, true, nil
}

func (r *pointsRepository) GetBalance(ctx context.Context, userID int64) (int64, error) {
	var balance int64

	err := r.pool.QueryRow(ctx, getBalanceSQL, userID).Scan(&balance)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}

		return 0, fmt.Errorf("get points balance: %w", err)
	}

	return balance, nil
}
