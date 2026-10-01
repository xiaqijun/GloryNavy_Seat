-- name: CreateFlow :exec
INSERT INTO eve_login_flows (state_hash, browser_hash, session_hash, verifier, requested_scopes) VALUES ($1, $2, $3, $4, $5);
-- name: SetFlowIntent :exec
UPDATE eve_login_flows SET intent=$2,target_user_id=$3,expected_character_id=$4 WHERE state_hash=$1;
-- name: ConsumeFlow :one
DELETE FROM eve_login_flows
WHERE state_hash = $1 AND browser_hash = $2 AND session_hash = $3 AND expires_at > now()
RETURNING verifier, requested_scopes, intent, target_user_id, expected_character_id;

-- name: RemoveUserFlows :exec
DELETE FROM eve_login_flows WHERE target_user_id=$1;
-- name: PruneFlows :exec
DELETE FROM eve_login_flows WHERE state_hash IN (
  SELECT state_hash FROM eve_login_flows WHERE expires_at <= now() LIMIT 1000
);
