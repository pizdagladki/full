package service

import (
	"context"
	"fmt"

	"github.com/pizdagladki/full/services/store/internal/api/domain"
	"github.com/pizdagladki/full/services/store/internal/api/repository"
)

type inventoryService struct {
	repo repository.InventoryRepository
}

// NewInventoryService returns an InventoryService wired to the given repository.
func NewInventoryService(repo repository.InventoryRepository) InventoryService {
	return &inventoryService{repo: repo}
}

func (s *inventoryService) ListInventory(ctx context.Context, userID int64) ([]domain.InventoryItem, error) {
	items, err := s.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("inventory service list: %w", err)
	}

	return items, nil
}
