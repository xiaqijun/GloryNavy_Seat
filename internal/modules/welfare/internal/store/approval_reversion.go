package store

import (
	"context"
	"encoding/json"
)

func LastCancellationRequest(ctx context.Context, db DB, id int64) (json.RawMessage, error) {
	var result json.RawMessage
	err := db.QueryRow(ctx, `SELECT result FROM welfare_audit WHERE case_id=$1 AND action='request_cancel' ORDER BY id DESC LIMIT 1`, id).Scan(&result)
	return result, err
}

// The caller holds the welfare publication lock. A merged batch owns its
// frozen aggregate; never roll back an individual item behind that batch.
func ReversionBatchExists(ctx context.Context, db DB, id int64) (bool, error) {
	var exists bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM welfare_contract_settlement_items WHERE source='welfare' AND source_id=$1)`, id).Scan(&exists)
	return exists, err
}

func ArchiveApprovalReversion(ctx context.Context, db DB, actor, request string, c Case, contracts json.RawMessage) error {
	_, err := db.Exec(ctx, `INSERT INTO welfare_approval_reversions(case_id,actor_id,request_key,previous_version,previous_state,snapshot,quota_spans,contracts)
 SELECT $1,$2::uuid,$3::uuid,$4,$5,to_jsonb(c),coalesce((SELECT jsonb_agg(to_jsonb(q) ORDER BY q.ordinal) FROM welfare_loss_quota_days q WHERE q.case_id=c.id),'[]'::jsonb),$6::jsonb FROM welfare_cases c WHERE c.id=$1`, c.ID, actor, request, c.Version, c.State, contracts)
	return err
}

func RemoveRevertedQuota(ctx context.Context, db DB, id int64) error {
	_, err := db.Exec(ctx, `DELETE FROM welfare_loss_quota_days WHERE case_id=$1`, id)
	return err
}

func LastReversionSnapshot(ctx context.Context, db DB, id int64) (json.RawMessage, error) {
	var detail json.RawMessage
	err := db.QueryRow(ctx, `SELECT snapshot->'detail' FROM welfare_approval_reversions WHERE case_id=$1 AND previous_state IN ('approved','executing') ORDER BY id DESC LIMIT 1`, id).Scan(&detail)
	return detail, err
}
