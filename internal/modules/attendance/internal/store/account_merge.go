package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
)

// MergeAccount is called inside identity's verified merge transaction via the host.
func MergeAccount(ctx context.Context, tx pgx.Tx, source, target string, apply bool) (json.RawMessage, error) {
	var summary json.RawMessage
	if err := tx.QueryRow(ctx, `SELECT jsonb_build_object('points',(SELECT coalesce(sum(points),0) FROM attendance_pap_awards WHERE account_id=$1),'entries',(SELECT count(*) FROM attendance_entries WHERE account_id=$1),'fingerprint',md5((SELECT coalesce(md5(string_agg(row_hash, '' ORDER BY row_hash)),'') FROM (SELECT md5(to_jsonb(t)::text) row_hash FROM attendance_entries t WHERE account_id IN($1::uuid,$2::uuid)) rows) || (SELECT coalesce(md5(string_agg(row_hash, '' ORDER BY row_hash)),'') FROM (SELECT md5(to_jsonb(t)::text) row_hash FROM attendance_pap_awards t WHERE account_id IN($1::uuid,$2::uuid)) rows) || (SELECT coalesce(md5(string_agg(row_hash, '' ORDER BY row_hash)),'') FROM (SELECT md5(to_jsonb(t)::text) row_hash FROM attendance_pap_ledger t WHERE account_id IN($1::uuid,$2::uuid)) rows) || (SELECT coalesce(md5(string_agg(row_hash, '' ORDER BY row_hash)),'') FROM (SELECT md5(to_jsonb(t)::text) row_hash FROM attendance_battle_tasks t WHERE account_id IN($1::uuid,$2::uuid)) rows)))`, source, target).Scan(&summary); err != nil {
		return nil, err
	}
	var pap json.RawMessage
	if err := tx.QueryRow(ctx, `SELECT jsonb_build_object('alliance',coalesce(jsonb_agg(to_jsonb(p) ORDER BY month,character_id),'[]'::jsonb)) FROM attendance_alliance_pap_snapshot p WHERE account_id IN($1::uuid,$2::uuid)`, source, target).Scan(&pap); err != nil {
		return nil, err
	}
	details := map[string]any{}
	if err := json.Unmarshal(summary, &details); err != nil {
		return nil, err
	}
	details["fingerprint"] = fmt.Sprintf("%x", sha256.Sum256(append(summary, pap...)))
	summary, _ = json.Marshal(details)
	if apply {
		if _, err := tx.Exec(ctx, `UPDATE attendance_alliance_pap_snapshot SET original_account_id=coalesce(original_account_id,account_id),account_id=$2 WHERE account_id=$1`, source, target); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE attendance_entries SET original_account_id=coalesce(original_account_id,account_id),account_id=$2 WHERE account_id=$1`, source, target); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE attendance_pap_awards SET original_account_id=coalesce(original_account_id,account_id),account_id=$2 WHERE account_id=$1`, source, target); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE attendance_pap_ledger SET original_account_id=coalesce(original_account_id,account_id),account_id=$2 WHERE account_id=$1`, source, target); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE attendance_battle_tasks SET original_account_id=coalesce(original_account_id,account_id),account_id=$2 WHERE account_id=$1`, source, target); err != nil {
			return nil, err
		}
	}
	return summary, nil
}
