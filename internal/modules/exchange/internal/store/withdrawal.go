package store

import "context"

// Called only while holding the account and redemption locks. Repeated starts
// retain the first operator/time and do not alter contract evidence.
func StartDelivery(ctx context.Context, db DBTX, id int64, actor string) error {
	_, err := db.Exec(ctx, `INSERT INTO exchange_deliveries(order_id,started_at,started_by) VALUES($1,now(),$2)
 ON CONFLICT(order_id) DO UPDATE SET started_at=coalesce(exchange_deliveries.started_at,EXCLUDED.started_at),started_by=CASE WHEN exchange_deliveries.started_at IS NULL THEN EXCLUDED.started_by ELSE exchange_deliveries.started_by END`, id, actor)
	return err
}
