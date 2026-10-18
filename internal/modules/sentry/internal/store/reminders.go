package store

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

// ReminderKeys returns only opaque references owned by this module.
func ReminderKeys(ctx context.Context, db *pgxpool.Pool, account, kind string, since time.Time) ([]string, error) {
	var query string
	switch kind {
	case "sentry_alert":
		query = `SELECT id::text||':'||updated_at::text FROM sentry_keys WHERE account_id=$1 AND status='sync_error' AND updated_at < now()-interval '15 minutes' AND $2::timestamptz IS NOT NULL`
	default:
		return nil, errors.New("unsupported reminder source")
	}
	rows, err := db.Query(ctx, query, account, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			return nil, err
		}
		out = append(out, key)
	}
	return out, rows.Err()
}
