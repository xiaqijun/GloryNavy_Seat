package store

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

// ReminderKeys returns only opaque references owned by this module.
func ReminderKeys(ctx context.Context, db *pgxpool.Pool, account, kind string, since time.Time) ([]string, error) {
	var query string
	switch kind {
	case "loan_result":
		query = `SELECT id::text||':'||state FROM loan_cases WHERE borrower_account_id=$1 AND updated_at >= $2 AND state IN ('awaiting_acceptance','approved','funding','active','settled','rejected','cancelled','defaulted','disputed')`
	case "guarantee_pending":
		query = `SELECT g.id::text||':'||g.state FROM loan_guarantees g JOIN loan_cases c ON c.id=g.case_id WHERE g.guarantor_account_id=$1 AND g.state IN ('invited','called','disputed') AND c.state NOT IN ('cancelled','rejected','settled') AND $2::timestamptz IS NOT NULL`
	case "loan_due":
		query = `SELECT i.id::text||':'||i.due_at::text FROM loan_installments i JOIN loan_cases c ON c.id=i.case_id WHERE c.borrower_account_id=$1 AND c.state IN ('active','defaulted') AND i.state IN ('pending','partial','overdue') AND i.due_at BETWEEN now() AND now()+interval '3 days' AND $2::timestamptz IS NOT NULL`
	case "loan_overdue":
		query = `SELECT i.id::text||':'||i.due_at::text FROM loan_installments i JOIN loan_cases c ON c.id=i.case_id WHERE c.borrower_account_id=$1 AND c.state IN ('active','defaulted') AND i.state IN ('pending','partial','overdue') AND i.due_at < now() AND $2::timestamptz IS NOT NULL`
	case "collateral_pending":
		query = `SELECT r.id::text FROM loan_collateral_requests r JOIN loan_cases c ON c.id=r.case_id WHERE c.borrower_account_id=$1 AND r.state='awaiting_delivery' AND c.state NOT IN ('cancelled','rejected','settled') AND $2::timestamptz IS NOT NULL`
	case "contribution_result":
		query = `SELECT id::text FROM loan_contributions WHERE account_id=$1 AND state='funded' AND funded_at >= $2`
	default:
		return nil, errors.New("unsupported reminder source")
	}
	rows, err := db.Query(ctx, query, account, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			return nil, err
		}
		out = append(out, key)
	}
	return out, rows.Err()
}
