package store

import "context"

func SetAuthorizationPurpose(ctx context.Context, db DBTX, stateHash []byte, purpose string) error {
	_, err := db.Exec(ctx, `UPDATE eve_login_flows SET purpose = $2 WHERE state_hash = $1`, stateHash, purpose)
	return err
}

// ConsumeFlowWithPurpose preserves the atomic single-use and browser/session checks.
func ConsumeFlowWithPurpose(ctx context.Context, db DBTX, arg ConsumeFlowParams) (ConsumeFlowRow, string, error) {
	row := db.QueryRow(ctx, `DELETE FROM eve_login_flows
WHERE state_hash = $1 AND browser_hash = $2 AND session_hash = $3 AND expires_at > now()
RETURNING verifier, requested_scopes, intent, target_user_id, expected_character_id, purpose`, arg.StateHash, arg.BrowserHash, arg.SessionHash)
	var flow ConsumeFlowRow
	var purpose string
	err := row.Scan(&flow.Verifier, &flow.RequestedScopes, &flow.Intent, &flow.TargetUserID, &flow.ExpectedCharacterID, &purpose)
	return flow, purpose, err
}

func MarkFullScopeGrant(ctx context.Context, db DBTX) error {
	_, err := db.Exec(ctx, `UPDATE eve_full_scope_grant_state SET full_grants_seen = true WHERE singleton`)
	return err
}
