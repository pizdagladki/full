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
