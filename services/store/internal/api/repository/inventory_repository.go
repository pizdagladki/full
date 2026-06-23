package repository

import (
	"context"
	"fmt"

	"github.com/pizdagladki/full/services/store/internal/api/domain"
)

type inventoryRepository struct {
	pool rowsQuerier
}

// NewInventoryRepository returns an InventoryRepository backed by pool.
// In production pass *pgxpool.Pool; in tests pass pgxmock.PgxPoolIface.
func NewInventoryRepository(pool rowsQuerier) InventoryRepository {
	return &inventoryRepository{pool: pool}
}

const listInventoryByUserSQL = `
SELECT product_id, quantity
FROM inventory
WHERE user_id = $1
ORDER BY product_id`

func (r *inventoryRepository) ListByUser(ctx context.Context, userID int64) ([]domain.InventoryItem, error) {
	rows, err := r.pool.Query(ctx, listInventoryByUserSQL, userID)
	if err != nil {
		return nil, fmt.Errorf("list inventory: %w", err)
	}
	defer rows.Close()

	items := make([]domain.InventoryItem, 0)

	for rows.Next() {
		var item domain.InventoryItem

		err = rows.Scan(&item.ProductID, &item.Quantity)
		if err != nil {
			return nil, fmt.Errorf("scan inventory item: %w", err)
		}

		items = append(items, item)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate inventory: %w", err)
	}

	return items, nil
}
