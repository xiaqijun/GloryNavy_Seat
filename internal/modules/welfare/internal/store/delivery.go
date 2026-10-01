package store

import (
	"context"
	"github.com/jackc/pgx/v5"
	"time"
)

func ScheduleDelivery(ctx context.Context, db DB, id int64, due time.Time) error {
	_, e := db.Exec(ctx, `UPDATE welfare_cases SET delivery_check_at=$2 WHERE id=$1`, id, due)
	return e
}
func DueDeliveries(ctx context.Context, db DB) ([]int64, error) {
	rows, e := db.Query(ctx, `SELECT id FROM welfare_cases WHERE ((state IN ('executing','cancel_requested') AND detail ? 'delivery') OR ((kind IN ('srp','solo','supercarrier','titan') OR left(kind,7)='growth_' OR left(kind,9)='activity_') AND state IN ('approved','executing','cancel_requested'))) AND (delivery_check_at IS NULL OR delivery_check_at<=now()) ORDER BY delivery_check_at NULLS FIRST,id LIMIT 100`)
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

// Pending PVP quotes are selected from persisted applications, so a restart or
// failed enqueue cannot leave a submitted case permanently unpriced.
func PendingValuations(ctx context.Context, db DB) ([]int64, error) {
	rows, e := db.Query(ctx, `SELECT id FROM welfare_cases WHERE kind='solo'
 AND state IN ('submitted','information','external')
 AND detail #>> '{valuation,state}'='pending' ORDER BY id LIMIT 100`)
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
func DeliveryAudit(ctx context.Context, db DB, actor string, v Case) error {
	_, e := db.Exec(ctx, `INSERT INTO welfare_audit(actor_id,request_key,fingerprint,case_id,action,note,result) VALUES($1,gen_random_uuid(),'delivery-check',$2,'delivery_check',$3,$4)`, actor, v.ID, "同步核验交付合同", v)
	return e
}

// CompleteMergedCaseTx records one aggregate contract against a case without
// trying to match the aggregate item list to this case's individual reward.
// The caller has already performed the frozen batch-level exact match.
func CompleteMergedCaseTx(ctx context.Context, db DB, actor string, v Case) error {
	tag, err := db.Exec(ctx, `UPDATE welfare_cases SET state='completed',detail=$2,version=version+1,updated_at=now() WHERE id=$1 AND state IN ('approved','executing','cancel_requested')`, v.ID, v.Detail)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return pgx.ErrNoRows
	}
	return DeliveryAudit(ctx, db, actor, v)
}
