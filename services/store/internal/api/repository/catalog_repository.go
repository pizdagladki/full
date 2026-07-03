package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/pizdagladki/full/services/store/internal/api/domain"
)

// rowsQuerier is the minimal pool interface required by catalogRepository.
// Both *pgxpool.Pool and pgxmock.PgxPoolIface satisfy this interface, allowing
// tests to inject a mock without a live database.
type rowsQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

type catalogRepository struct {
	pool rowsQuerier
}

// NewCatalogRepository returns a CatalogRepository backed by pool.
// In production pass *pgxpool.Pool; in tests pass pgxmock.PgxPoolIface.
func NewCatalogRepository(pool rowsQuerier) CatalogRepository {
	return &catalogRepository{pool: pool}
}

const listProductsAllSQL = `
SELECT id, kind, tier, name, price_cents, is_free, points_price
FROM products
ORDER BY id`

const listProductsByKindSQL = `
SELECT id, kind, tier, name, price_cents, is_free, points_price
FROM products
WHERE kind = $1
ORDER BY id`

func (r *catalogRepository) ListProducts(ctx context.Context, kind *string) ([]domain.Product, error) {
	var (
		rows pgx.Rows
		err  error
	)

	if kind == nil {
		rows, err = r.pool.Query(ctx, listProductsAllSQL)
	} else {
		rows, err = r.pool.Query(ctx, listProductsByKindSQL, *kind)
	}

	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	defer rows.Close()

	products := make([]domain.Product, 0)

	for rows.Next() {
		var p domain.Product

		err = rows.Scan(&p.ID, &p.Kind, &p.Tier, &p.Name, &p.PriceCents, &p.IsFree, &p.PointsPrice)
		if err != nil {
			return nil, fmt.Errorf("scan product: %w", err)
		}

		products = append(products, p)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate products: %w", err)
	}

	return products, nil
}
