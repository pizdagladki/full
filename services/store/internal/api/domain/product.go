// Package domain holds the store service domain models, DTOs, and enums.
package domain

import "errors"

// KindDistraction is the "distraction" product kind.
const KindDistraction = "distraction"

// KindEdit is the "edit" product kind.
const KindEdit = "edit"

// ErrInvalidKind is returned when an unrecognized product kind is supplied.
var ErrInvalidKind = errors.New("invalid product kind")

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
