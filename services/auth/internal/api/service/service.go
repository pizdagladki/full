// Package service holds the auth service business logic (orchestrating
// repositories and external integrations such as Google OAuth). Service
// interfaces are added here by downstream resource slices via the new-resource
// skill.
package service

//go:generate mockgen -source=service.go -destination=mocks/service_mock.go -package=mocks

import (
	"context"
	"errors"

	"github.com/pizdagladki/full/services/auth/internal/api/domain"
)

// ErrInvalidCode is returned by OAuthExchanger when the authorization code is
// invalid, expired, or the exchange call fails. The delivery layer maps it to
// HTTP 401.
var ErrInvalidCode = errors.New("oauth: invalid or expired code")

// ErrSessionNotFound is returned by SessionStore when the session id is
// missing, expired, or has been deleted.
var ErrSessionNotFound = errors.New("session: not found or expired")

// GoogleUser holds the minimal profile data returned by the Google OpenID
// userinfo endpoint after a successful code exchange.
type GoogleUser struct {
	Sub   string
	Email string
}

// OAuthExchanger abstracts the Google authorization-code → user-profile flow so
// it can be mocked in tests without a live network.
type OAuthExchanger interface {
	ExchangeCode(ctx context.Context, code string) (GoogleUser, error)
}

// SessionStore manages Redis-backed sessions.
type SessionStore interface {
	// Create persists a new session for userID and returns its opaque id.
	Create(ctx context.Context, userID int64) (sessionID string, err error)
	// Get returns the userID stored under sessionID, or ErrSessionNotFound.
	Get(ctx context.Context, sessionID string) (userID int64, err error)
	// Delete removes the session.
	Delete(ctx context.Context, sessionID string) error
}

// AuthService is the top-level auth business-logic contract.
type AuthService interface {
	// LoginGoogle exchanges an OAuth code for a profile, upserts the user, and
	// creates a session. It propagates ErrInvalidCode on a bad code.
	LoginGoogle(ctx context.Context, code string) (sessionID string, user domain.User, err error)
	// Authenticate resolves a sessionID to the owning user.
	// Returns an error (wrapping ErrSessionNotFound or repo.ErrNotFound) when
	// the session is missing, expired, or the user no longer exists.
	Authenticate(ctx context.Context, sessionID string) (domain.User, error)
}
