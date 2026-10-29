package store

import (
	"context"
	"time"
)

type RepaymentObligation struct {
	CaseID, InstallmentID, OutstandingMinor int64
	DueAt                                   time.Time
}

// OtherRepaymentObligations returns the borrower's unpaid installments; an
// approved/funding case has no fixed schedule yet and is reported as unknown.
func OtherRepaymentObligations(ctx context.Context, db DBTX, borrower string, excludeCase int64) ([]RepaymentObligation, bool, error) {
	var unknown bool
	if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM loan_cases c WHERE c.borrower_account_id=$1::uuid AND c.id<>$2 AND c.state IN ('approved','funding','active','defaulted') AND NOT EXISTS(SELECT 1 FROM loan_installments i WHERE i.case_id=c.id))`, borrower, excludeCase).Scan(&unknown); err != nil {
		return nil, false, err
	}
	rows, err := db.Query(ctx, `SELECT c.id,i.id,i.due_at,(i.principal_minor-i.paid_principal_minor+i.interest_minor-i.paid_interest_minor)::bigint FROM loan_cases c JOIN loan_installments i ON i.case_id=c.id WHERE c.borrower_account_id=$1::uuid AND c.id<>$2 AND c.state IN ('approved','funding','active','defaulted') AND i.state<>'paid' ORDER BY i.due_at,c.id,i.id`, borrower, excludeCase)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := []RepaymentObligation{}
	for rows.Next() {
		var v RepaymentObligation
		if err = rows.Scan(&v.CaseID, &v.InstallmentID, &v.DueAt, &v.OutstandingMinor); err != nil {
			return nil, false, err
		}
		out = append(out, v)
	}
	return out, unknown, rows.Err()
}

// PoolCapitalAndBorrowed uses cash plus unpaid funded principal; reservations
// are already included in cash and must not be added a second time.
func PoolCapitalAndBorrowed(ctx context.Context, db DBTX, poolID int64, borrower string) (capital, borrowed int64, err error) {
	err = db.QueryRow(ctx, `WITH obligations AS (
 SELECT c.borrower_account_id,c.state,greatest(c.principal_minor::numeric-COALESCE((SELECT sum(i.paid_principal_minor) FROM loan_installments i WHERE i.case_id=c.id),0),0) outstanding
 FROM loan_cases c WHERE c.pool_id=$1 AND c.state IN ('approved','funding','active','defaulted')
)
SELECT (COALESCE((SELECT sum(delta_minor) FROM loan_pool_ledger WHERE pool_id=$1),0)+COALESCE((SELECT sum(outstanding) FROM obligations WHERE state IN ('active','defaulted')),0))::bigint,
 COALESCE((SELECT sum(outstanding) FROM obligations WHERE borrower_account_id=$2::uuid),0)::bigint`, poolID, borrower).Scan(&capital, &borrowed)
	return
}

func LockCreditPolicy(ctx context.Context, db DBTX) error {
	var singleton bool
	return db.QueryRow(ctx, `SELECT singleton FROM loan_credit_policy WHERE singleton FOR SHARE`).Scan(&singleton)
}
