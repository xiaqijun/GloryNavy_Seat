package store

import (
	"context"
	"encoding/json"
	"time"
)

// DeliveryContracts reads only scoped, already synchronised contract evidence.
func DeliveryContracts(ctx context.Context, db DBTX, kind string, owner, recipient, corp, id int64, since time.Time, lock bool) ([]json.RawMessage, error) {
	suffix := ""
	if lock {
		// Lock both evidence owners before the snapshot; item workers publish under the detail lock.
		if _, e := db.Exec(ctx, `SELECT 1 FROM eve_contracts WHERE owner_kind=$1 AND owner_id=$2 AND contract_id=$3 FOR SHARE`, kind, owner, id); e != nil {
			return nil, e
		}
		if _, e := db.Exec(ctx, `SELECT 1 FROM eve_contract_details WHERE owner_kind=$1 AND owner_id=$2 AND contract_id=$3 AND part='items' FOR SHARE`, kind, owner, id); e != nil {
			return nil, e
		}
	}
	rows, err := db.Query(ctx, `SELECT jsonb_build_object(
 'id',c.contract_id::text,'owner_kind',c.owner_kind,'owner_id',c.owner_id::text,
 'type',c.contract_type,'status',c.status,'checked_at',c.checked_at,'payload',c.payload,
 'items_ready',EXISTS(SELECT 1 FROM eve_contract_details d WHERE d.owner_kind=c.owner_kind AND d.owner_id=c.owner_id AND d.contract_id=c.contract_id AND d.part='items' AND d.state='ready'),
 'items',coalesce((SELECT jsonb_agg(jsonb_build_object('record_id',i.record_id::text,'type_id',i.type_id::text,'quantity',i.quantity::text,'included',i.is_included,'singleton',i.is_singleton,'raw_quantity',i.raw_quantity::text) ORDER BY i.record_id) FROM eve_contract_items i WHERE i.owner_kind=c.owner_kind AND i.owner_id=c.owner_id AND i.contract_id=c.contract_id),'[]'::jsonb))
 FROM eve_contracts c WHERE c.owner_kind=$1 AND c.owner_id=$2 AND c.in_scope
 AND ($5::bigint=0 OR c.contract_id=$5)
 AND ($5::bigint<>0 OR (c.contract_type='item_exchange' AND c.payload->>'assignee_id'=$3::bigint::text
 AND c.payload->>'issuer_corporation_id'=$4::bigint::text AND (c.payload->>'date_issued')::timestamptz >= $6))
 ORDER BY c.contract_id DESC LIMIT 50`+suffix, kind, owner, recipient, corp, id, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var b json.RawMessage
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
