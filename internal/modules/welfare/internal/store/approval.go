package store

import (
	"context"
	"glorynavy.local/seat/internal/platform/reviewqueue"
)

func Approval(ctx context.Context, db DB, scope []byte, user string, f reviewqueue.Filter, p reviewqueue.Position, limit int) (reviewqueue.Page, error) {
	return reviewqueue.Read(ctx, db, `
 SELECT c.id,
 CASE WHEN $11='history' THEN coalesce(a.created_at,c.updated_at) ELSE coalesce((SELECT max(created_at) FROM welfare_audit WHERE case_id=c.id AND action IN ('apply','resubmit')),c.created_at) END moment,
 CASE WHEN c.state IN ('submitted','cancel_requested','external') THEN 'pending'
 WHEN c.state='information' THEN 'information'
 WHEN c.state IN ('approved','executing') AND coalesce(c.detail->>'payment_status','')<>'awaiting_acceptance' THEN CASE WHEN coalesce(c.detail->>'payment_status','') IN ('multiple_contracts','mismatch','issuer_unverified','contract_claimed','contract_unavailable','snapshot_required') THEN 'exceptions' ELSE 'fulfillment' END ELSE 'history' END bucket,
 (a.id IS NOT NULL OR c.state IN ('completed','cancelled','rejected','reversed') OR c.detail->>'payment_status'='awaiting_acceptance') history,
 coalesce(a.actor_id::text,'') processed_by,
 jsonb_build_object('id',c.id::text,'version',c.version::text,'account_id',c.account_id::text,'corporation_id',c.corporation_id::text,
 'kind',c.kind,'state',c.state,'status',coalesce(c.detail->>'payment_status',''),'recipient',coalesce(c.detail->>'character_name',''),
 'title',coalesce(nullif(c.detail->'rule'->>'project_name',''),c.detail->'loss_evidence'->>'ship_name',''),
 'reference',c.settlement_reference,'amount_minor',c.award_minor,'unit','ISK','action',coalesce(a.action,''),'actions','[]'::jsonb,
 'payload',jsonb_build_object('id',c.id::text,'reference',c.settlement_reference,'account_id',c.account_id::text,'corporation_id',c.corporation_id::text,'kind',c.kind,'state',c.state,'version',c.version::text,'detail',c.detail,'award_minor',c.award_minor,'claim_keys',c.claim_keys,'created_at',c.created_at,'updated_at',c.updated_at)) item
 FROM welfare_cases c
 LEFT JOIN LATERAL (SELECT id,created_at,actor_id,action FROM welfare_audit WHERE case_id=c.id AND (action IN ('approve','reject','information','approve_cancel','reject_cancel','complete','release_coins','cancel','void') OR action='delivery_check' AND result->>'state'='completed') AND (NOT $10 OR actor_id::text=$2 AND action<>'delivery_check') ORDER BY created_at DESC,id DESC LIMIT 1) a ON true
 WHERE c.kind<>'grant' AND EXISTS(SELECT 1 FROM jsonb_array_elements($1::jsonb) s WHERE s->>'corporation'=c.corporation_id::text AND s->>'account'=c.account_id::text)
 `, scope, user, f, p, limit, "welfare")
}
