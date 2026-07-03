package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/pizdagladki/full/services/media/internal/api/domain"
)

// kingClipRepository reuses the clipQuerier pool interface (defined in
// clip_repository.go): the required pgx surface (Query/QueryRow/Exec) is
// identical for both resources, so a second, structurally-duplicate interface
// would be redundant.
type kingClipRepository struct {
	pool clipQuerier
}

// NewKingClipRepository returns a KingClipRepository backed by pool.
// In production pass *pgxpool.Pool; in tests pass pgxmock.PgxPoolIface.
func NewKingClipRepository(pool clipQuerier) KingClipRepository {
	return &kingClipRepository{pool: pool}
}

const createKingClipSQL = `
INSERT INTO king_clips (user_id, hill_type, object_key, blink_ts_ms, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, user_id, hill_type, object_key, blink_ts_ms, created_at, expires_at`

func (r *kingClipRepository) Create(ctx context.Context, clip domain.KingClip) (domain.KingClip, error) {
	row := r.pool.QueryRow(ctx, createKingClipSQL,
		clip.UserID, clip.HillType, clip.ObjectKey, clip.BlinkTsMs, clip.ExpiresAt,
	)

	created, err := scanKingClip(row)
	if err != nil {
		return domain.KingClip{}, fmt.Errorf("create king clip: %w", err)
	}

	return created, nil
}

const getCurrentKingClipSQL = `
SELECT id, user_id, hill_type, object_key, blink_ts_ms, created_at, expires_at
FROM king_clips
WHERE hill_type = $1 AND expires_at > now()
ORDER BY created_at DESC, id DESC
LIMIT 1`

func (r *kingClipRepository) GetCurrent(ctx context.Context, hillType string) (domain.KingClip, error) {
	row := r.pool.QueryRow(ctx, getCurrentKingClipSQL, hillType)

	clip, err := scanKingClip(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.KingClip{}, ErrKingClipNotFound
		}

		return domain.KingClip{}, fmt.Errorf("get current king clip: %w", err)
	}

	return clip, nil
}

const getKingClipByIDSQL = `
SELECT id, user_id, hill_type, object_key, blink_ts_ms, created_at, expires_at
FROM king_clips
WHERE id = $1`

func (r *kingClipRepository) GetByID(ctx context.Context, id int64) (domain.KingClip, error) {
	row := r.pool.QueryRow(ctx, getKingClipByIDSQL, id)

	clip, err := scanKingClip(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.KingClip{}, ErrKingClipNotFound
		}

		return domain.KingClip{}, fmt.Errorf("get king clip by id: %w", err)
	}

	return clip, nil
}

const deleteKingClipSQL = `DELETE FROM king_clips WHERE id = $1 RETURNING object_key`

func (r *kingClipRepository) Delete(ctx context.Context, id int64) (string, error) {
	row := r.pool.QueryRow(ctx, deleteKingClipSQL, id)

	var objectKey string

	err := row.Scan(&objectKey)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrKingClipNotFound
		}

		return "", fmt.Errorf("delete king clip: %w", err)
	}

	return objectKey, nil
}

const deleteSupersededByHillSQL = `
DELETE FROM king_clips
WHERE hill_type = $1 AND id < $2
RETURNING object_key`

func (r *kingClipRepository) DeleteSupersededByHill(
	ctx context.Context, hillType string, keepID int64,
) ([]string, error) {
	rows, err := r.pool.Query(ctx, deleteSupersededByHillSQL, hillType, keepID)
	if err != nil {
		return nil, fmt.Errorf("delete superseded king clips: %w", err)
	}
	defer rows.Close()

	return scanObjectKeys(rows)
}

const deleteExpiredKingClipsSQL = `DELETE FROM king_clips WHERE expires_at <= now() RETURNING object_key`

func (r *kingClipRepository) DeleteExpired(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, deleteExpiredKingClipsSQL)
	if err != nil {
		return nil, fmt.Errorf("delete expired king clips: %w", err)
	}
	defer rows.Close()

	return scanObjectKeys(rows)
}

// scanKingClip scans a single king_clips row into a domain.KingClip.
func scanKingClip(row pgx.Row) (domain.KingClip, error) {
	var c domain.KingClip

	err := row.Scan(
		&c.ID, &c.UserID, &c.HillType, &c.ObjectKey, &c.BlinkTsMs, &c.CreatedAt, &c.ExpiresAt,
	)
	if err != nil {
		return domain.KingClip{}, err
	}

	return c, nil
}

// scanObjectKeys drains rows expected to contain a single object_key column.
func scanObjectKeys(rows pgx.Rows) ([]string, error) {
	keys := make([]string, 0)

	for rows.Next() {
		var key string

		err := rows.Scan(&key)
		if err != nil {
			return nil, fmt.Errorf("scan object_key: %w", err)
		}

		keys = append(keys, key)
	}

	err := rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate object keys: %w", err)
	}

	return keys, nil
}
