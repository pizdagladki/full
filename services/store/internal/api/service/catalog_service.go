package service

import (
	"context"
	"fmt"

	"github.com/pizdagladki/full/services/store/internal/api/domain"
	"github.com/pizdagladki/full/services/store/internal/api/repository"
)

type catalogService struct {
	repo repository.CatalogRepository
}

// NewCatalogService returns a CatalogService wired to the given repository.
func NewCatalogService(repo repository.CatalogRepository) CatalogService {
	return &catalogService{repo: repo}
}

func (s *catalogService) ListCatalog(ctx context.Context, kind *string) ([]domain.Product, error) {
	if kind != nil && !domain.ValidKind(*kind) {
		return nil, domain.ErrInvalidKind
	}

	products, err := s.repo.ListProducts(ctx, kind)
	if err != nil {
		return nil, fmt.Errorf("catalog service list: %w", err)
	}

	return products, nil
}
