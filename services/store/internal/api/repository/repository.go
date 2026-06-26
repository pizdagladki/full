// Package repository holds the store service data access (hand-written SQL via
// pgx, mapping rows to domain models). Repository interfaces for catalog items
// and inventory are added here by downstream resource slices via the
// new-resource skill.
package repository

//go:generate mockgen -source=repository.go -destination=mocks/repository_mock.go -package=mocks

import (
	"context"
	"errors"

	"github.com/pizdagladki/full/services/store/internal/api/domain"
)

// PurchaseRepository is the data-access contract for purchase and inventory
// grant operations.
type PurchaseRepository interface {
	// GetProduct fetches a product by ID. Returns ErrProductNotFound when absent.
	GetProduct(ctx context.Context, productID int64) (*domain.Product, error)
	// IsOwned reports whether userID already has quantity > 0 for productID.
	IsOwned(ctx context.Context, userID, productID int64) (bool, error)
	// CreatePurchase inserts a pending purchase record and returns its ID.
	CreatePurchase(ctx context.Context, p domain.Purchase) (int64, error)
	// WebhookEventExists reports whether a purchase with stripe_event_id=eventID
	// already exists (idempotency guard).
	WebhookEventExists(ctx context.Context, eventID string) (bool, error)
	// FindByProviderRef returns the purchase whose provider_ref matches
	// providerRef (i.e. Stripe PaymentIntent ID).
	FindByProviderRef(ctx context.Context, providerRef string) (*domain.Purchase, error)
	// ConfirmAndGrant atomically marks the purchase as paid (set stripe_event_id)
	// and upserts inventory: distractions increment quantity; edits use DO NOTHING.
	ConfirmAndGrant(ctx context.Context, providerRef, eventID, kind string, userID, productID int64) error
}

// ErrSessionNotFound is returned by SessionRepository when the session key is
// absent from Redis or has expired.
var ErrSessionNotFound = errors.New("session: not found or expired")

// CatalogRepository is the data-access contract for the products table.
type CatalogRepository interface {
	// ListProducts returns all products, optionally filtered by kind when kind
	// is non-nil. Results are ordered by id ascending.
	ListProducts(ctx context.Context, kind *string) ([]domain.Product, error)
}

// InventoryRepository is the data-access contract for the inventory table.
type InventoryRepository interface {
	// ListByUser returns all inventory rows for userID, ordered by product_id.
	ListByUser(ctx context.Context, userID int64) ([]domain.InventoryItem, error)
}

// SessionRepository resolves Redis session keys to user IDs.
type SessionRepository interface {
	// UserIDBySession returns the user_id stored under session:<sessionID>.
	// Returns ErrSessionNotFound when the key is absent or has expired.
	UserIDBySession(ctx context.Context, sessionID string) (int64, error)
}
