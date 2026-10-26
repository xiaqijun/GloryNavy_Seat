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
	case "feedback_update":
		query = `SELECT m.id::text FROM feedback_messages m JOIN feedback_tickets t ON t.id=m.ticket_id WHERE t.account_id=$1 AND m.actor<>$1 AND m.created_at >= $2`
	case "feedback_pending":
		query = `SELECT CASE WHEN m.previous_status IN ('awaiting_confirmation','resolved','closed','withdrawn') THEN 'returned:'||m.id::text ELSE t.id::text END
 FROM feedback_tickets t LEFT JOIN LATERAL (
 SELECT id,lag(status) OVER(ORDER BY id) previous_status FROM feedback_messages WHERE ticket_id=t.id AND status<>'' ORDER BY id DESC LIMIT 1
 ) m ON true WHERE t.status IN ('submitted','in_progress') AND t.account_id<>$1 AND $2::timestamptz IS NOT NULL`
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
