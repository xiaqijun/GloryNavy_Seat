package store

import "context"

// SecurityCommitments separates effective coverage from amounts awaiting action.
// Clamp sums in SQL numeric arithmetic; malformed historic over-allocation must
// neither overflow nor create a new gap. Pending amounts never approve a loan.
func SecurityCommitments(ctx context.Context, db DBTX, caseID, principal, excludeGuarantee int64) (effective, pending int64, legacyPending bool, err error) {
	err = db.QueryRow(ctx, `SELECT
        LEAST($2::numeric,COALESCE(sum(amount) FILTER (WHERE effective),0))::bigint,
        LEAST($2::numeric,COALESCE(sum(amount) FILTER (WHERE NOT effective),0))::bigint,
        EXISTS(SELECT 1 FROM loan_collateral_requests WHERE case_id=$1 AND state='awaiting_delivery' AND coverage_minor=0)
        FROM (
          SELECT amount_minor::numeric AS amount,state IN ('accepted','active') AS effective
            FROM loan_guarantees WHERE case_id=$1 AND id<>$3 AND state IN ('invited','accepted','active')
          UNION ALL
          SELECT covered_minor::numeric,state IN ('approved','held')
            FROM loan_collateral WHERE case_id=$1 AND state IN ('proposed','approved','held')
          UNION ALL
          SELECT coverage_minor::numeric,false FROM loan_collateral_requests
            WHERE case_id=$1 AND state='awaiting_delivery'
        ) amounts`, caseID, principal, excludeGuarantee).Scan(&effective, &pending, &legacyPending)
	return
}
