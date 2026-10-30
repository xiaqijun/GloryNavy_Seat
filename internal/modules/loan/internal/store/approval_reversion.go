package store

import "context"

// RevertReview is called after locking the pool and case. It preserves all
// payment and security records; historical decisions remain in loan_audit.
func RevertReview(ctx context.Context, db DBTX, id, version int64) (Case, error) {
	var updated int64
	err := db.QueryRow(ctx, `UPDATE loan_cases SET state='submitted',reviewer_id=NULL,review_note='',accepted_at=NULL,version=version+1,updated_at=now() WHERE id=$1 AND version=$2 AND state IN ('approved','rejected') RETURNING id`, id, version).Scan(&updated)
	if err != nil {
		return Case{}, err
	}
	return GetCase(ctx, db, updated)
}

func HasPaymentIntents(ctx context.Context, db DBTX, id int64) (bool, error) {
	var exists bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM loan_payment_intents WHERE case_id=$1)`, id).Scan(&exists)
	return exists, err
}
