package store

import "context"

// LastDecisionWasRejectedCancellation is called while the redemption row is
// locked; a later request, delivery action or withdrawal invalidates it.
func LastDecisionWasRejectedCancellation(ctx context.Context, db DBTX, id int64) (bool, error) {
	var ok bool
	err := db.QueryRow(ctx, `SELECT coalesce((SELECT kind='decision' AND payload->>'state'='pending' FROM exchange_shop_audit WHERE target_id=$1 AND kind IN ('decision','delivery') ORDER BY id DESC LIMIT 1),false)`, id).Scan(&ok)
	return ok, err
}
