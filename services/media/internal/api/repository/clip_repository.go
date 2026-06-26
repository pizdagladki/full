package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/pizdagladki/full/services/media/internal/api/domain"
)

// clipQuerier is the minimal pool interface required by clipRepository.
// Both *pgxpool.Pool and pgxmock.PgxPoolIface satisfy this interface, allowing
// tests to inject a mock without a live database.
type clipQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type clipRepository struct {
	pool clipQuerier
}

// NewClipRepository returns a ClipRepository backed by pool.
// In production pass *pgxpool.Pool; in tests pass pgxmock.PgxPoolIface.
func NewClipRepository(pool clipQuerier) ClipRepository {
	return &clipRepository{pool: pool}
}

const createClipSQL = `
INSERT INTO clips (user_id, object_key, mode, result, size_bytes, content_type)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, user_id, object_key, mode, result, size_bytes, content_type,
          created_at, mp4_object_key, conversion_status`

func (r *clipRepository) Create(ctx context.Context, clip domain.Clip) (domain.Clip, error) {
	row := r.pool.QueryRow(ctx, createClipSQL,
		clip.UserID, clip.ObjectKey, clip.Mode, clip.Result, clip.SizeBytes, clip.ContentType,
	)

	var created domain.Clip

	err := row.Scan(
		&created.ID,
		&created.UserID,
		&created.ObjectKey,
		&created.Mode,
		&created.Result,
		&created.SizeBytes,
		&created.ContentType,
		&created.CreatedAt,
		&created.MP4ObjectKey,
		&created.ConversionStatus,
	)
	if err != nil {
		return domain.Clip{}, fmt.Errorf("create clip: %w", err)
	}

	return created, nil
}

const listByUserSQL = `
SELECT id, user_id, object_key, mode, result, size_bytes, content_type, created_at, mp4_object_key, conversion_status
FROM clips
WHERE user_id = $1
ORDER BY created_at DESC, id DESC`

func (r *clipRepository) ListByUser(ctx context.Context, userID int64) ([]domain.Clip, error) {
	rows, err := r.pool.Query(ctx, listByUserSQL, userID)
	if err != nil {
		return nil, fmt.Errorf("list clips by user: %w", err)
	}
	defer rows.Close()

	clips := make([]domain.Clip, 0)

	for rows.Next() {
		var c domain.Clip

		err = rows.Scan(
			&c.ID, &c.UserID, &c.ObjectKey, &c.Mode, &c.Result,
			&c.SizeBytes, &c.ContentType, &c.CreatedAt,
			&c.MP4ObjectKey, &c.ConversionStatus,
		)
		if err != nil {
			return nil, fmt.Errorf("scan clip: %w", err)
		}

		clips = append(clips, c)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate clips: %w", err)
	}

	return clips, nil
}

const getByIDSQL = `
SELECT id, user_id, object_key, mode, result, size_bytes, content_type, created_at, mp4_object_key, conversion_status
FROM clips
WHERE id = $1`

func (r *clipRepository) GetByID(ctx context.Context, id int64) (domain.Clip, error) {
	row := r.pool.QueryRow(ctx, getByIDSQL, id)

	var c domain.Clip

	err := row.Scan(
		&c.ID, &c.UserID, &c.ObjectKey, &c.Mode, &c.Result,
		&c.SizeBytes, &c.ContentType, &c.CreatedAt,
		&c.MP4ObjectKey, &c.ConversionStatus,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Clip{}, ErrClipNotFound
		}

		return domain.Clip{}, fmt.Errorf("get clip by id: %w", err)
	}

	return c, nil
}

const deleteOldestBeyondLimitSQL = `
DELETE FROM clips
WHERE id IN (
    SELECT id FROM clips WHERE user_id = $1 ORDER BY created_at DESC, id DESC OFFSET $2
)
RETURNING object_key`

func (r *clipRepository) DeleteOldestBeyondLimit(ctx context.Context, userID int64, limit int) ([]string, error) {
	rows, err := r.pool.Query(ctx, deleteOldestBeyondLimitSQL, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("delete oldest clips: %w", err)
	}
	defer rows.Close()

	keys := make([]string, 0)

	for rows.Next() {
		var key string

		err = rows.Scan(&key)
		if err != nil {
			return nil, fmt.Errorf("scan object_key: %w", err)
		}

		keys = append(keys, key)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate deleted keys: %w", err)
	}

	return keys, nil
}

const updateConversionSQL = `UPDATE clips SET mp4_object_key = $2, conversion_status = $3 WHERE id = $1`

func (r *clipRepository) UpdateConversion(ctx context.Context, id int64, mp4Key, status string) error {
	_, err := r.pool.Exec(ctx, updateConversionSQL, id, mp4Key, status)
	if err != nil {
		return fmt.Errorf("update conversion: %w", err)
	}

	return nil
}
