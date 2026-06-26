package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/pizdagladki/full/services/auth/internal/api/domain"
)

// ErrConsentNotFound is returned by GetByUserID when no consent row exists for
// the given user.
var ErrConsentNotFound = errors.New("consent: not found")

type consentRepository struct {
	pool rowQuerier
}

// NewConsentRepository returns a ConsentRepository backed by pool. In
// production pass *pgxpool.Pool; in tests pass pgxmock.PgxPoolIface — both
// satisfy the private rowQuerier interface.
func NewConsentRepository(pool rowQuerier) ConsentRepository {
	return &consentRepository{pool: pool}
}

const consentUpsertSQL = `
INSERT INTO user_consents (user_id, is_adult, consent_recording, consent_tos, accepted_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (user_id) DO UPDATE
    SET is_adult           = EXCLUDED.is_adult,
        consent_recording  = EXCLUDED.consent_recording,
        consent_tos        = EXCLUDED.consent_tos,
        accepted_at        = now()
RETURNING is_adult, consent_recording, consent_tos, accepted_at`

func (r *consentRepository) Upsert(ctx context.Context, userID int64, c domain.Consent) (domain.Consent, error) {
	row := r.pool.QueryRow(ctx, consentUpsertSQL, userID, c.IsAdult, c.ConsentRecording, c.ConsentTos)

	var out domain.Consent

	err := row.Scan(&out.IsAdult, &out.ConsentRecording, &out.ConsentTos, &out.AcceptedAt)
	if err != nil {
		return domain.Consent{}, fmt.Errorf("upsert consent: %w", err)
	}

	return out, nil
}

const consentGetByUserIDSQL = `
SELECT is_adult, consent_recording, consent_tos, accepted_at
FROM user_consents
WHERE user_id = $1`

func (r *consentRepository) GetByUserID(ctx context.Context, userID int64) (domain.Consent, error) {
	row := r.pool.QueryRow(ctx, consentGetByUserIDSQL, userID)

	var c domain.Consent

	err := row.Scan(&c.IsAdult, &c.ConsentRecording, &c.ConsentTos, &c.AcceptedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Consent{}, ErrConsentNotFound
		}

		return domain.Consent{}, fmt.Errorf("get consent by user id: %w", err)
	}

	return c, nil
}
