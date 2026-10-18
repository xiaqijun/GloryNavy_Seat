package store

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
)

func AuthorizationReminderKeys(ctx context.Context, db *pgxpool.Pool, id int64, owner []byte) ([]string, error) {
	rows, err := db.Query(ctx, `SELECT character_id::text||':'||grant_generation::text FROM eve_credentials WHERE character_id=$1 AND owner_hash=$2 AND state='reauthorize'`, id, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return reminderRows(rows)
}
func SyncReminderKeys(ctx context.Context, db *pgxpool.Pool) ([]string, error) {
	rows, err := db.Query(ctx, `SELECT t.id::text||':'||t.generation::text||':'||coalesce(t.last_success_at::text,'initial') FROM eve_sync_targets t JOIN eve_credentials c ON c.character_id=t.character_id AND c.grant_generation=t.generation WHERE t.failures>=3 AND t.state IN ('failed','deferred') AND coalesce(t.last_success_at,t.updated_at)<now()-interval '1 hour' AND c.state<>'reauthorize'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return reminderRows(rows)
}
func ContractReminderKeys(ctx context.Context, db *pgxpool.Pool, id int64, owner []byte) ([]string, error) {
	rows, err := db.Query(ctx, `SELECT x.contract_id::text FROM eve_contracts x JOIN eve_credentials c ON c.character_id=x.owner_id JOIN eve_sync_targets t ON t.character_id=c.character_id AND t.resource='character_contracts' AND t.generation=c.grant_generation WHERE x.owner_kind='character' AND x.owner_id=$1 AND c.owner_hash=$2 AND c.state IN ('ready','retry') AND t.last_success_at>now()-interval '2 hours' AND t.state<>'blocked' AND x.in_scope AND x.status='outstanding' AND (x.payload->>'assignee_id')::bigint=$1 AND x.source_character_id=c.character_id AND x.source_generation=c.grant_generation AND (x.payload->>'date_expired')::timestamptz>now()`, id, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return reminderRows(rows)
}

type reminderRowReader interface {
	Next() bool
	Scan(...any) error
	Err() error
}

func reminderRows(rows reminderRowReader) ([]string, error) {
	out := []string{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}
