package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/pizdagladki/full/services/auth/internal/api/domain"
)

// ErrNotFound is returned by GetByID when no user row matches the given id.
var ErrNotFound = errors.New("user: not found")

// rowQuerier is the minimal pool interface required by userRepository.
// Both *pgxpool.Pool and pgxmock.PgxPoolIface satisfy this interface, which
// allows tests to inject a mock without depending on a live database.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type userRepository struct {
	pool rowQuerier
}

// NewUserRepository returns a UserRepository backed by pool. In production pass
// *pgxpool.Pool; in tests pass pgxmock.PgxPoolIface — both satisfy rowQuerier.
func NewUserRepository(pool rowQuerier) UserRepository {
	return &userRepository{pool: pool}
}

const upsertSQL = `
INSERT INTO users (google_sub, email)
VALUES ($1, $2)
ON CONFLICT (google_sub) DO UPDATE SET email = EXCLUDED.email
RETURNING id, google_sub, email, created_at`

func (r *userRepository) UpsertByGoogleSub(ctx context.Context, googleSub, email string) (domain.User, error) {
	row := r.pool.QueryRow(ctx, upsertSQL, googleSub, email)

	var u domain.User

	err := row.Scan(&u.ID, &u.GoogleSub, &u.Email, &u.CreatedAt)
	if err != nil {
		return domain.User{}, fmt.Errorf("upsert user: %w", err)
	}

	return u, nil
}

const getByIDSQL = `SELECT id, google_sub, email, created_at FROM users WHERE id = $1`

func (r *userRepository) GetByID(ctx context.Context, id int64) (domain.User, error) {
	row := r.pool.QueryRow(ctx, getByIDSQL, id)

	var u domain.User

	err := row.Scan(&u.ID, &u.GoogleSub, &u.Email, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, ErrNotFound
		}

		return domain.User{}, fmt.Errorf("get user by id: %w", err)
	}

	return u, nil
}
