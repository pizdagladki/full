// Package domain holds the store service domain models, DTOs, and enums.
package domain

import "errors"

// KindDistraction is the "distraction" product kind.
const KindDistraction = "distraction"

// KindEdit is the "edit" product kind.
const KindEdit = "edit"

// ErrInvalidKind is returned when an unrecognized product kind is supplied.
var ErrInvalidKind = errors.New("invalid product kind")

// ErrNotGrantable is returned when a rewarded-video grant is attempted on a
// product that is not a free distraction (a paid item, or a non-distraction
// kind such as an edit).
var ErrNotGrantable = errors.New("product is not a grantable free distraction")

// ErrRateLimited is returned when a user has exceeded the configured
// rewarded-video grant rate limit.
var ErrRateLimited = errors.New("rewarded grant rate limit exceeded")

// ValidKind reports whether s is a recognized product kind.
func ValidKind(s string) bool {
	return s == KindDistraction || s == KindEdit
}

// Product is the domain model for a store product.
type Product struct {
	ID          int64
	Kind        string
	Tier        *int
	Name        string
	PriceCents  int
	IsFree      bool
	PointsPrice *int64 // nil = money-only; non-nil = purchasable with points
}

// InventoryItem is the domain model for an item in a user's inventory.
type InventoryItem struct {
	ProductID int64
	Quantity  int
}
