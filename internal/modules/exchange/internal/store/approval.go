package store

import (
	"context"
	"encoding/json"
	"glorynavy.local/seat/internal/platform/reviewqueue"
)

func ApprovalRecipients(ctx context.Context, db DBTX) ([]int64, error) {
	rows, e := db.Query(ctx, `SELECT DISTINCT recipient_id FROM exchange_redemptions`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if e = rows.Scan(&id); e != nil {
			return nil, e
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
func ApprovalHistory(ctx context.Context, db DBTX, id int64) ([]json.RawMessage, error) {
	rows, e := db.Query(ctx, `SELECT jsonb_build_object('action',CASE WHEN kind='decision' THEN payload->>'state' ELSE kind END,'note',coalesce(payload->>'note',''),'actor_id',actor_id,'created_at',created_at) FROM exchange_shop_audit WHERE target_id=$1 AND kind IN ('claim','decision','delivery') ORDER BY id`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var r json.RawMessage
		if e = rows.Scan(&r); e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func Approval(ctx context.Context, db DBTX, scope []byte, user string, f reviewqueue.Filter, p reviewqueue.Position, limit int) (reviewqueue.Page, error) {
	return reviewqueue.Read(ctx, db, `
 SELECT r.id, CASE WHEN $11='history' THEN coalesce(a.created_at,r.decided_at,r.created_at) ELSE r.created_at END moment,
 CASE WHEN r.state='cancel_requested' THEN 'pending' WHEN r.state='pending' AND coalesce(d.status,'')='awaiting_acceptance' THEN 'history' WHEN r.state='pending' THEN CASE WHEN coalesce(d.status,'') IN ('mismatch','multiple_contracts','issuer_unverified','contract_claimed','evidence_unavailable') THEN 'exceptions' ELSE 'fulfillment' END ELSE 'history' END bucket,
 (a.id IS NOT NULL OR r.state IN ('fulfilled','cancelled') OR coalesce(d.status,'')='awaiting_acceptance') history,
 coalesce(a.actor_id::text,'') processed_by,
 jsonb_build_object('id',r.id::text,'version',r.version::text,'account_id',r.account_id::text,'corporation_id','','kind','exchange','state',r.state,'status',coalesce(d.status,'waiting_contract'),
 'recipient',r.recipient_name,'title',r.reward_name,'reference',r.settlement_reference,'amount_minor',r.coins_minor,'unit','coin','action',coalesce(a.kind,''),'actions','[]'::jsonb,
 'payload',jsonb_build_object('id',r.id::text,'version',r.version::text,'reference',r.settlement_reference,'type_id',r.type_id::text,'name',r.reward_name,'quantity',r.quantity,'recipient_id',r.recipient_id::text,'recipient_name',r.recipient_name,'coins_minor',r.coins_minor,'isk_per_coin',r.isk_per_coin,'isk_value',r.isk_value,'state',r.state,'note',r.note,'created_at',r.created_at,'content',r.reward_content,'delivery',jsonb_build_object('status',coalesce(d.status,'waiting_contract'),'contracts',coalesce(d.evidence,'[]'::jsonb),'checked_at',d.checked_at))) item
 FROM exchange_redemptions r
 LEFT JOIN exchange_deliveries d ON d.order_id=r.id
 LEFT JOIN LATERAL (SELECT id,created_at,actor_id,CASE WHEN kind='decision' THEN coalesce(payload->>'state',kind) ELSE kind END kind FROM exchange_shop_audit WHERE target_id=r.id AND (kind='delivery' OR kind='decision' AND payload->>'state' IN ('pending','cancelled')) AND (NOT $10 OR actor_id::text=$2 AND kind<>'delivery') ORDER BY created_at DESC,id DESC LIMIT 1) a ON true
 WHERE EXISTS(SELECT 1 FROM jsonb_array_elements($1::jsonb) s WHERE s->>'recipient'=r.recipient_id::text AND s->>'account'=r.account_id::text)
 `, scope, user, f, p, limit, "exchange")
}
