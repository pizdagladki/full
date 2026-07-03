package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/pizdagladki/full/services/store/internal/api/domain"
)

// rewardedPoolIface is the minimal pool interface required by
// rewardedRepository: both GetProduct and GrantFreeDistraction use QueryRow
// (a plain SELECT and an upsert with RETURNING, respectively — neither needs
// a transaction). Both *pgxpool.Pool and pgxmock.PgxPoolIface satisfy this.
type rewardedPoolIface interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type rewardedRepository struct {
	pool rewardedPoolIface
}

// NewRewardedRepository returns a RewardedRepository backed by pool.
// In production pass *pgxpool.Pool; in tests pass pgxmock.PgxPoolIface.
func NewRewardedRepository(pool rewardedPoolIface) RewardedRepository {
	return &rewardedRepository{pool: pool}
}

func (r *rewardedRepository) GetProduct(ctx context.Context, productID int64) (*domain.Product, error) {
	var p domain.Product

	err := r.pool.QueryRow(ctx, getProductSQL, productID).
		Scan(&p.ID, &p.Kind, &p.Tier, &p.Name, &p.PriceCents, &p.IsFree, &p.PointsPrice)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrProductNotFound
		}

		return nil, fmt.Errorf("get product: %w", err)
	}

	return &p, nil
}

const grantFreeDistractionSQL = `
INSERT INTO inventory (user_id, product_id, quantity)
VALUES ($1, $2, 1)
ON CONFLICT (user_id, product_id) DO UPDATE
SET quantity = inventory.quantity + 1, updated_at = now()
RETURNING quantity`

func (r *rewardedRepository) GrantFreeDistraction(ctx context.Context, userID, productID int64) (int, error) {
	var quantity int

	err := r.pool.QueryRow(ctx, grantFreeDistractionSQL, userID, productID).Scan(&quantity)
	if err != nil {
		return 0, fmt.Errorf("grant free distraction: %w", err)
	}

	return quantity, nil
}
