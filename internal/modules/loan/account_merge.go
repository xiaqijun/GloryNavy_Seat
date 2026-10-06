package loan

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
)

// Loan obligations and funded contributions follow the target account. The
// original account is retained on every moved row for audit and reconciliation.
func (s *Service) MergeAccountTx(ctx context.Context, tx pgx.Tx, source, target string, apply bool) (json.RawMessage, error) {
	var contributions, cases, guarantees, collateral, evaluations int64
	for _, q := range []struct {
		sql string
		out *int64
	}{
		{`SELECT count(*) FROM loan_contributions WHERE account_id=$1`, &contributions},
		{`SELECT count(*) FROM loan_cases WHERE borrower_account_id=$1`, &cases},
		{`SELECT count(*) FROM loan_guarantees WHERE guarantor_account_id=$1`, &guarantees},
		{`SELECT count(*) FROM loan_collateral WHERE owner_account_id=$1`, &collateral},
		{`SELECT count(*) FROM loan_credit_evaluations WHERE account_id=$1`, &evaluations},
	} {
		if err := tx.QueryRow(ctx, q.sql, source).Scan(q.out); err != nil {
			return nil, err
		}
	}
	if apply {
		if _, err := tx.Exec(ctx, `UPDATE loan_contributions SET original_account_id=coalesce(original_account_id,account_id),account_id=$2 WHERE account_id=$1`, source, target); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE loan_cases SET original_account_id=coalesce(original_account_id,borrower_account_id),borrower_account_id=$2 WHERE borrower_account_id=$1`, source, target); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE loan_guarantees SET original_account_id=coalesce(original_account_id,guarantor_account_id),guarantor_account_id=$2 WHERE guarantor_account_id=$1`, source, target); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE loan_collateral SET original_account_id=coalesce(original_account_id,owner_account_id),owner_account_id=$2 WHERE owner_account_id=$1`, source, target); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE loan_pools SET original_account_id=coalesce(original_account_id,lender_user_id),lender_user_id=$2 WHERE lender_user_id=$1`, source, target); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE loan_credit_evaluations SET original_account_id=coalesce(original_account_id,account_id),account_id=$2 WHERE account_id=$1`, source, target); err != nil {
			return nil, err
		}
	}
	return json.Marshal(map[string]any{"contributions": contributions, "borrower_cases": cases, "guarantees": guarantees, "collateral": collateral, "credit_evaluations": evaluations})
}
