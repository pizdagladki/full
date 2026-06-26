package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/pizdagladki/full/services/store/internal/api/domain"
)

// purchasePoolIface is the minimal pool interface required by purchaseRepository.
// It extends rowsQuerier with QueryRow, Exec, and Begin so the repository can
// execute transactions. Both *pgxpool.Pool and pgxmock.PgxPoolIface satisfy
// this interface.
type purchasePoolIface interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Begin(ctx context.Context) (pgx.Tx, error)
}

type purchaseRepository struct {
	pool purchasePoolIface
}

// NewPurchaseRepository returns a PurchaseRepository backed by pool.
// In production pass *pgxpool.Pool; in tests pass pgxmock.PgxPoolIface.
func NewPurchaseRepository(pool purchasePoolIface) PurchaseRepository {
	return &purchaseRepository{pool: pool}
}

const getProductSQL = `
SELECT id, kind, tier, name, price_cents, is_free
FROM products
WHERE id = $1`

func (r *purchaseRepository) GetProduct(ctx context.Context, productID int64) (*domain.Product, error) {
	var p domain.Product

	err := r.pool.QueryRow(ctx, getProductSQL, productID).
		Scan(&p.ID, &p.Kind, &p.Tier, &p.Name, &p.PriceCents, &p.IsFree)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrProductNotFound
		}

		return nil, fmt.Errorf("get product: %w", err)
	}

	return &p, nil
}

const isOwnedSQL = `
SELECT EXISTS(SELECT 1 FROM inventory WHERE user_id = $1 AND product_id = $2 AND quantity > 0)`

func (r *purchaseRepository) IsOwned(ctx context.Context, userID, productID int64) (bool, error) {
	var owned bool

	err := r.pool.QueryRow(ctx, isOwnedSQL, userID, productID).Scan(&owned)
	if err != nil {
		return false, fmt.Errorf("check ownership: %w", err)
	}

	return owned, nil
}

const createPurchaseSQL = `
INSERT INTO purchases (user_id, product_id, provider, provider_ref, amount_cents, status)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id`

func (r *purchaseRepository) CreatePurchase(ctx context.Context, p domain.Purchase) (int64, error) {
	var id int64

	err := r.pool.QueryRow(ctx, createPurchaseSQL,
		p.UserID, p.ProductID, p.Provider, p.ProviderRef, p.AmountCents, p.Status,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create purchase: %w", err)
	}

	return id, nil
}

const webhookEventExistsSQL = `
SELECT EXISTS(SELECT 1 FROM purchases WHERE stripe_event_id = $1)`

func (r *purchaseRepository) WebhookEventExists(ctx context.Context, eventID string) (bool, error) {
	var exists bool

	err := r.pool.QueryRow(ctx, webhookEventExistsSQL, eventID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check webhook event: %w", err)
	}

	return exists, nil
}

const findByProviderRefSQL = `
SELECT id, user_id, product_id, provider, provider_ref, amount_cents, status
FROM purchases
WHERE provider_ref = $1
LIMIT 1`

func (r *purchaseRepository) FindByProviderRef(ctx context.Context, providerRef string) (*domain.Purchase, error) {
	var p domain.Purchase

	err := r.pool.QueryRow(ctx, findByProviderRefSQL, providerRef).
		Scan(&p.ID, &p.UserID, &p.ProductID, &p.Provider, &p.ProviderRef, &p.AmountCents, &p.Status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("find purchase by provider_ref: %w", pgx.ErrNoRows)
		}

		return nil, fmt.Errorf("find purchase by provider_ref: %w", err)
	}

	return &p, nil
}

const confirmPurchaseSQL = `
UPDATE purchases SET status = 'paid', stripe_event_id = $1 WHERE provider_ref = $2 AND stripe_event_id IS NULL`

const upsertInventoryDistractionSQL = `
INSERT INTO inventory (user_id, product_id, quantity)
VALUES ($1, $2, 1)
ON CONFLICT (user_id, product_id) DO UPDATE
SET quantity = inventory.quantity + 1, updated_at = now()`

const upsertInventoryEditSQL = `
INSERT INTO inventory (user_id, product_id, quantity)
VALUES ($1, $2, 1)
ON CONFLICT (user_id, product_id) DO NOTHING`

func (r *purchaseRepository) ConfirmAndGrant(
	ctx context.Context,
	providerRef, eventID, kind string,
	userID, productID int64,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	tag, err := tx.Exec(ctx, confirmPurchaseSQL, eventID, providerRef)
	if err != nil {
		return fmt.Errorf("confirm purchase: %w", err)
	}

	if tag.RowsAffected() == 0 {
		// Already processed by a concurrent delivery of the same event.
		return tx.Commit(ctx)
	}

	var upsertSQL string
	if kind == domain.KindDistraction {
		upsertSQL = upsertInventoryDistractionSQL
	} else {
		upsertSQL = upsertInventoryEditSQL
	}

	_, err = tx.Exec(ctx, upsertSQL, userID, productID)
	if err != nil {
		return fmt.Errorf("upsert inventory: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}
