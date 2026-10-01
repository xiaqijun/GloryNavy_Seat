package store

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"time"
)

type Delivery struct {
	Status    string          `json:"status"`
	Evidence  json.RawMessage `json:"contracts"`
	CheckedAt *time.Time      `json:"checked_at"`
}

func EnsureDelivery(ctx context.Context, db DBTX, id int64) error {
	_, e := db.Exec(ctx, `INSERT INTO exchange_deliveries(order_id) VALUES($1) ON CONFLICT DO NOTHING`, id)
	return e
}
func ReadDelivery(ctx context.Context, db DBTX, id int64) (Delivery, error) {
	v := Delivery{Status: "waiting_contract", Evidence: json.RawMessage(`[]`)}
	e := db.QueryRow(ctx, `SELECT status,evidence,checked_at FROM exchange_deliveries WHERE order_id=$1`, id).Scan(&v.Status, &v.Evidence, &v.CheckedAt)
	if e == pgx.ErrNoRows {
		return v, nil
	}
	return v, e
}
func SaveDelivery(ctx context.Context, db DBTX, id int64, status string, evidence any) error {
	raw, e := json.Marshal(evidence)
	if e != nil {
		return e
	}
	_, e = db.Exec(ctx, `INSERT INTO exchange_deliveries(order_id,status,evidence,checked_at,check_at) VALUES($1,$2,$3,now(),now()+interval '5 minutes') ON CONFLICT(order_id) DO UPDATE SET status=$2,evidence=$3,checked_at=now(),check_at=now()+interval '5 minutes'`, id, status, raw)
	return e
}
func DueDeliveries(ctx context.Context, db DBTX) ([]int64, error) {
	rows, e := db.Query(ctx, `SELECT d.order_id FROM exchange_deliveries d JOIN exchange_redemptions r ON r.id=d.order_id WHERE r.state IN ('pending','cancel_requested') AND d.check_at<=now() ORDER BY d.check_at,d.order_id LIMIT 100`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if e = rows.Scan(&id); e != nil {
			return nil, e
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
func ScheduleDelivery(ctx context.Context, db DBTX, id int64, at time.Time) error {
	_, e := db.Exec(ctx, `UPDATE exchange_deliveries SET check_at=$2 WHERE order_id=$1`, id, at)
	return e
}
func DeliveryAudit(ctx context.Context, db DBTX, id int64, actor string, evidence any) error {
	raw, e := json.Marshal(evidence)
	if e != nil {
		return e
	}
	_, e = db.Exec(ctx, `INSERT INTO exchange_shop_audit(actor_id,request_key,kind,target_id,fingerprint,payload) VALUES($1,gen_random_uuid(),'delivery',$2,md5($3::text),$3::jsonb)`, actor, id, string(raw))
	return e
}

func MarkDeliveryUnavailable(ctx context.Context, db DBTX, id int64) error {
	_, e := db.Exec(ctx, `UPDATE exchange_deliveries d SET status='evidence_unavailable',checked_at=now() FROM exchange_redemptions r WHERE d.order_id=$1 AND r.id=d.order_id AND r.state IN ('pending','cancel_requested')`, id)
	return e
}
