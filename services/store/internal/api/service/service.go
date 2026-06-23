// Package service holds the store service business logic (orchestrating
// repositories and external integrations). Service interfaces for catalog and
// inventory are added here by downstream resource slices via the new-resource
// skill.
package service

//go:generate mockgen -source=service.go -destination=mocks/service_mock.go -package=mocks

import (
	"context"

	"github.com/pizdagladki/full/services/store/internal/api/domain"
)

// CatalogService is the business-logic contract for catalog operations.
type CatalogService interface {
	// ListCatalog returns all products, optionally filtered by kind.
	// Returns domain.ErrInvalidKind when kind is non-nil and not recognized.
	ListCatalog(ctx context.Context, kind *string) ([]domain.Product, error)
}

// InventoryService is the business-logic contract for inventory operations.
type InventoryService interface {
	// ListInventory returns the caller's owned product ids and quantities.
	ListInventory(ctx context.Context, userID int64) ([]domain.InventoryItem, error)
}

// SessionService resolves a session cookie value to a user ID.
type SessionService interface {
	// ResolveSession returns the user_id stored under the session cookie value.
	// Returns the repository.ErrSessionNotFound sentinel when absent/expired.
	ResolveSession(ctx context.Context, sessionID string) (int64, error)
}
