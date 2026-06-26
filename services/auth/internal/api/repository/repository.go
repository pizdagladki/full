// Package repository holds the auth service data access (hand-written SQL via
// pgx, mapping rows to domain models). Repository interfaces are added here by
// downstream resource slices via the new-resource skill.
package repository

//go:generate mockgen -source=repository.go -destination=mocks/repository_mock.go -package=mocks

import (
	"context"

	"github.com/pizdagladki/full/services/auth/internal/api/domain"
)

// UserRepository is the data-access contract for users.
type UserRepository interface {
	// UpsertByGoogleSub inserts a new user or updates the email on Google sub
	// conflict, returning the current row.
	UpsertByGoogleSub(ctx context.Context, googleSub, email string) (domain.User, error)

	// GetByID returns the user with the given primary-key id. It wraps
	// pgx.ErrNoRows in a not-found sentinel when no row matches.
	GetByID(ctx context.Context, id int64) (domain.User, error)
}

// ConsentRepository is the data-access contract for user registration consents.
type ConsentRepository interface {
	// Upsert inserts or updates the consent row for userID (ON CONFLICT (user_id))
	// and returns the persisted state with the DB-generated accepted_at timestamp.
	Upsert(ctx context.Context, userID int64, c domain.Consent) (domain.Consent, error)

	// GetByUserID returns the consent row for userID. It returns
	// ErrConsentNotFound when no row exists for that user.
	GetByUserID(ctx context.Context, userID int64) (domain.Consent, error)
}
