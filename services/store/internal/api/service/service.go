// Package service holds the store service business logic (orchestrating
// repositories and external integrations such as Stripe payments via a
// PaymentProvider interface). Service interfaces for catalog, purchases, and
// inventory are added here by downstream resource slices via the new-resource
// skill.
package service
