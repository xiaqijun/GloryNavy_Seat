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
	case "pap_change":
		query = `SELECT id::text FROM attendance_pap_ledger WHERE account_id=$1 AND created_at >= $2`
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
