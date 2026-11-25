package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ConnectionSettings struct {
	Origin     string
	Ciphertext []byte
	Revision   int64
	UpdatedAt  time.Time
}

func ReadConnectionSettings(ctx context.Context, pool *pgxpool.Pool) (ConnectionSettings, error) {
	var row ConnectionSettings
	err := pool.QueryRow(ctx, `SELECT origin, token_ciphertext, revision, updated_at FROM sentry_connection_settings WHERE singleton`).
		Scan(&row.Origin, &row.Ciphertext, &row.Revision, &row.UpdatedAt)
	return row, err
}

func WriteConnectionSettings(ctx context.Context, pool *pgxpool.Pool, origin string, encrypted []byte, revision int64) (ConnectionSettings, error) {
	var row ConnectionSettings
	err := pool.QueryRow(ctx, `
		INSERT INTO sentry_connection_settings(singleton, origin, token_ciphertext, revision)
		SELECT true, $1, $2, $3::bigint + 1
		WHERE $3 = 0 OR EXISTS(SELECT 1 FROM sentry_connection_settings WHERE singleton AND revision = $3)
		ON CONFLICT(singleton) DO UPDATE SET origin = excluded.origin,
			token_ciphertext = excluded.token_ciphertext, revision = excluded.revision, updated_at = now()
		WHERE sentry_connection_settings.revision = $3
		RETURNING origin, revision, updated_at`, origin, encrypted, revision).
		Scan(&row.Origin, &row.Revision, &row.UpdatedAt)
	return row, err
}
