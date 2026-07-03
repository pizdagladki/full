package domain

import "errors"

// PurchaseStatusPending is the initial status when a purchase is created.
const PurchaseStatusPending = "pending"

// PurchaseStatusPaid is the status after successful payment confirmation.
const PurchaseStatusPaid = "paid"

// ProviderStripe identifies the Stripe payment provider.
const ProviderStripe = "stripe"

// PayWithMoney is the (default) money/Stripe purchase path.
const PayWithMoney = "money"

// PayWithPoints is the points-balance purchase path.
const PayWithPoints = "points"

// ReasonShopSpend identifies points spent on a store purchase.
const ReasonShopSpend = "shop_spend"

var (
	// ErrProductNotFound is returned when the requested product does not exist.
	ErrProductNotFound = errors.New("product not found")

	// ErrAlreadyOwned is returned when a user tries to purchase an edit they already own.
	ErrAlreadyOwned = errors.New("product already owned")

	// ErrInvalidWebhook is returned when the webhook payload or signature is invalid.
	ErrInvalidWebhook = errors.New("invalid webhook")

	// ErrMoneyOnly is returned when a points purchase is attempted on a product
	// whose points_price is null.
	ErrMoneyOnly = errors.New("product is money-only")

	// ErrInsufficientPoints is returned when a user's points balance is lower
	// than the product's points_price.
	ErrInsufficientPoints = errors.New("insufficient points")
)

// Purchase is the domain model for a product purchase record.
type Purchase struct {
	ID          int64
	UserID      int64
	ProductID   int64
	Provider    string
	ProviderRef string // Stripe PaymentIntent ID
	AmountCents int
	Status      string
}
