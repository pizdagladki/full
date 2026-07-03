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

// PaymentProvider is the abstraction over a payment gateway (Stripe today;
// an RF provider can be swapped in without changing purchase logic).
type PaymentProvider interface {
	// CreatePaymentIntent creates a payment intent for the given product and
	// amount. Returns the client secret (for the frontend) and the payment
	// intent ID (stored as provider_ref).
	CreatePaymentIntent(
		ctx context.Context,
		productID int64,
		amountCents int,
	) (clientSecret, paymentIntentID string, err error)
	// VerifyWebhook validates the raw Stripe webhook payload and signature.
	// Returns the event ID, payment intent ID, whether the payment succeeded,
	// and any verification error.
	VerifyWebhook(payload []byte, sigHeader string) (eventID, paymentIntentID string, succeeded bool, err error)
}

// PurchaseService is the business-logic contract for purchase operations.
type PurchaseService interface {
	// InitiatePurchase creates a Stripe PaymentIntent and a pending purchase
	// record. Returns the client secret for the frontend to complete payment.
	// Returns ErrProductNotFound when the product does not exist.
	// Returns ErrAlreadyOwned when the user already owns an edit product.
	InitiatePurchase(ctx context.Context, userID, productID int64) (clientSecret string, err error)
	// HandleWebhook processes an incoming Stripe webhook event. It verifies the
	// signature, checks idempotency, and calls ConfirmAndGrant on success.
	// Non-success events are silently acknowledged (nil error).
	HandleWebhook(ctx context.Context, payload []byte, sigHeader string) error
	// PurchaseWithPoints spends points on a product: atomically debits the
	// user's points balance and grants inventory. Returns ErrProductNotFound
	// when the product does not exist, ErrMoneyOnly when the product has no
	// points_price, ErrAlreadyOwned when the user already owns an edit
	// product, and ErrInsufficientPoints when the balance is too low.
	PurchaseWithPoints(ctx context.Context, userID, productID int64) (newBalance int64, err error)
}

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

// PointsService is the business-logic contract for points earn/read
// operations.
type PointsService interface {
	// Credit resolves the earn amount (an explicit positive in.Delta, or the
	// config-driven amount for in.Reason), then appends a ledger row and
	// updates the balance atomically. Idempotent by (user_id, reason, ref_id):
	// a duplicate reference returns the existing balance unchanged. Returns
	// domain.ErrInvalidCredit when reason is empty or the resolved delta is
	// not positive.
	Credit(ctx context.Context, in domain.PointsCredit) (balance int64, err error)
	// GetBalance returns the user's points balance, preferring the Redis
	// cache and falling back to Postgres (the source of truth) on a miss.
	GetBalance(ctx context.Context, userID int64) (int64, error)
}

// RewardedService is the business-logic contract for the rewarded-video
// free-distraction grant.
type RewardedService interface {
	// GrantFreeDistraction grants one free distraction into userID's
	// inventory after checking eligibility and the rate limit, and returns
	// the resulting inventory quantity. Returns domain.ErrProductNotFound
	// when the product does not exist, domain.ErrNotGrantable when it is not
	// a free distraction (a paid item, or a non-distraction kind), and
	// domain.ErrRateLimited when the caller has exceeded the configured
	// per-user rate limit.
	GrantFreeDistraction(ctx context.Context, userID, productID int64) (newQuantity int, err error)
}
