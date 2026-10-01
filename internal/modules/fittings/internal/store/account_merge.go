package store

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
)

// MergeAccount is called inside identity's verified merge transaction via the host.
func MergeAccount(ctx context.Context, tx pgx.Tx, source, target string, apply bool) (json.RawMessage, error) {
	var sending bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fittings_game_saves WHERE account_id IN($1::uuid,$2::uuid) AND state='sending')`, source, target).Scan(&sending); err != nil {
		return nil, err
	}
	if sending {
		return nil, fmt.Errorf("game save in progress")
	}
	var summary json.RawMessage
	if err := tx.QueryRow(ctx, `SELECT jsonb_build_object('drafts',(SELECT count(*) FROM fittings_drafts WHERE account_id=$1),'fingerprint',md5((SELECT coalesce(md5(string_agg(row_hash, '' ORDER BY row_hash)),'') FROM (SELECT md5(to_jsonb(t)::text) row_hash FROM fittings_drafts t WHERE account_id IN($1::uuid,$2::uuid)) rows) || (SELECT coalesce(md5(string_agg(row_hash, '' ORDER BY row_hash)),'') FROM (SELECT md5(to_jsonb(t)::text) row_hash FROM fittings_game_saves t WHERE account_id IN($1::uuid,$2::uuid)) rows)))`, source, target).Scan(&summary); err != nil {
		return nil, err
	}
	if apply {
		if _, err := tx.Exec(ctx, `UPDATE fittings_drafts SET original_account_id=coalesce(original_account_id,account_id),account_id=$2 WHERE account_id=$1`, source, target); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE fittings_game_saves SET original_account_id=coalesce(original_account_id,account_id),account_id=$2 WHERE account_id=$1`, source, target); err != nil {
			return nil, err
		}
	}
	return summary, nil
}
