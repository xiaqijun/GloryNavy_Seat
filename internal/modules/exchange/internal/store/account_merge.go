package store

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
)

// MergeAccount is called inside identity's verified merge transaction via the host.
func MergeAccount(ctx context.Context, tx pgx.Tx, source, target string, apply bool) (json.RawMessage, error) {
	var summary json.RawMessage
	if err := tx.QueryRow(ctx, `SELECT jsonb_build_object('coins_minor',(SELECT coalesce(sum(delta),0) FROM exchange_coin_ledger WHERE account_id=$1),'orders',(SELECT count(*) FROM exchange_redemptions WHERE account_id=$1),'alert_grants',(SELECT count(*) FROM exchange_alert_grants WHERE account_id=$1),'alert_charges',(SELECT count(*) FROM exchange_alert_charges WHERE account_id=$1),'target_coins_minor',(SELECT coalesce(sum(delta),0) FROM exchange_coin_ledger WHERE account_id=$2),'fingerprint',md5((SELECT coalesce(md5(string_agg(row_hash, '' ORDER BY row_hash)),'') FROM (SELECT md5(to_jsonb(t)::text) row_hash FROM exchange_source_awards t WHERE account_id IN($1::uuid,$2::uuid)) rows) || (SELECT coalesce(md5(string_agg(row_hash, '' ORDER BY row_hash)),'') FROM (SELECT md5(to_jsonb(t)::text) row_hash FROM exchange_coin_ledger t WHERE account_id IN($1::uuid,$2::uuid)) rows) || (SELECT coalesce(md5(string_agg(row_hash, '' ORDER BY row_hash)),'') FROM (SELECT md5(to_jsonb(t)::text) row_hash FROM exchange_redemptions t WHERE account_id IN($1::uuid,$2::uuid)) rows) || (SELECT coalesce(md5(string_agg(row_hash, '' ORDER BY row_hash)),'') FROM (SELECT md5(to_jsonb(t)::text) row_hash FROM exchange_alert_grants t WHERE account_id IN($1::uuid,$2::uuid)) rows) || (SELECT coalesce(md5(string_agg(row_hash, '' ORDER BY row_hash)),'') FROM (SELECT md5(to_jsonb(t)::text) row_hash FROM exchange_alert_charges t WHERE account_id IN($1::uuid,$2::uuid)) rows)))`, source, target).Scan(&summary); err != nil {
		return nil, err
	}
	if apply {
		if _, err := tx.Exec(ctx, `UPDATE exchange_source_awards SET original_account_id=coalesce(original_account_id,account_id),account_id=$2 WHERE account_id=$1`, source, target); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE exchange_coin_ledger SET original_account_id=coalesce(original_account_id,account_id),account_id=$2 WHERE account_id=$1`, source, target); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE exchange_redemptions SET original_account_id=coalesce(original_account_id,account_id),account_id=$2 WHERE account_id=$1`, source, target); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE exchange_alert_grants SET original_account_id=coalesce(original_account_id,account_id),account_id=$2 WHERE account_id=$1`, source, target); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE exchange_alert_charges SET original_account_id=coalesce(original_account_id,account_id),account_id=$2 WHERE account_id=$1`, source, target); err != nil {
			return nil, err
		}
	}
	return summary, nil
}
