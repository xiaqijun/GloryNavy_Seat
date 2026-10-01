-- name: ResetTokenObservation :exec
INSERT INTO eve_token_observations(character_id,generation,observed_since,access_expires_at)
SELECT c.character_id,c.grant_generation,clock_timestamp(),sqlc.arg(expires_at)::timestamptz FROM eve_credentials c WHERE c.character_id=sqlc.arg(character_id)
ON CONFLICT(character_id) DO UPDATE SET generation=excluded.generation,observed_since=excluded.observed_since,
access_expires_at=excluded.access_expires_at,last_used_at=NULL,reuse_count=0,last_refresh_attempt_at=NULL,last_refresh_success_at=NULL,
last_refresh_reason='',refresh_successes=0,refresh_failures=0,consecutive_failures=0,
last_request_at=NULL,last_request_status=0,last_request_reason='',network_requests=0,cache_hits=0,rate_limit_waits=0,request_failures=0;

-- name: ObserveTokenUse :exec
UPDATE eve_token_observations SET observed_since=coalesce(observed_since,clock_timestamp()),
access_expires_at=sqlc.arg(expires_at),last_used_at=clock_timestamp(),reuse_count=reuse_count+sqlc.arg(reused)::bigint
WHERE character_id=sqlc.arg(character_id) AND generation=sqlc.arg(generation);

-- name: ObserveTokenRefresh :exec
UPDATE eve_token_observations SET observed_since=coalesce(observed_since,sqlc.arg(attempted_at)::timestamptz),
last_refresh_attempt_at=sqlc.arg(attempted_at),last_refresh_reason=sqlc.arg(reason),
last_refresh_success_at=CASE WHEN sqlc.arg(success)::boolean THEN clock_timestamp() ELSE last_refresh_success_at END,
refresh_successes=refresh_successes+CASE WHEN sqlc.arg(success) THEN 1 ELSE 0 END,
refresh_failures=refresh_failures+CASE WHEN sqlc.arg(success) THEN 0 ELSE 1 END,
consecutive_failures=CASE WHEN sqlc.arg(success) THEN 0 ELSE consecutive_failures+1 END
WHERE character_id=sqlc.arg(character_id) AND generation=sqlc.arg(generation);

-- name: InsertTokenEvent :exec
INSERT INTO eve_token_events(character_id,generation,outcome,reason,duration_ms)
SELECT c.character_id,c.grant_generation,sqlc.arg(outcome),sqlc.arg(reason),sqlc.arg(duration_ms)::bigint
FROM eve_credentials c WHERE c.character_id=sqlc.arg(character_id) AND c.grant_generation=sqlc.arg(generation);

-- name: ObserveTokenRequest :exec
UPDATE eve_token_observations o SET observed_since=coalesce(observed_since,clock_timestamp()),
last_request_at=clock_timestamp(),last_request_status=sqlc.arg(http_status),last_request_reason=sqlc.arg(reason),
network_requests=network_requests+sqlc.arg(network)::bigint,cache_hits=cache_hits+sqlc.arg(cache_hit)::bigint,
rate_limit_waits=rate_limit_waits+sqlc.arg(limited)::bigint,request_failures=request_failures+sqlc.arg(failed)::bigint
WHERE o.character_id=sqlc.arg(character_id) AND o.generation=sqlc.arg(generation)
AND EXISTS(SELECT 1 FROM eve_credentials c WHERE c.character_id=o.character_id AND c.grant_generation=o.generation AND c.state<>'reauthorize');

-- name: ListTokenObservations :many
SELECT c.character_id,c.state AS credential_state,c.grant_generation,c.scopes,
coalesce((SELECT display_name FROM eve_sync_targets t WHERE t.character_id=c.character_id ORDER BY t.id LIMIT 1),'')::text AS display_name,
o.* FROM eve_credentials c JOIN eve_token_observations o USING(character_id)
WHERE c.character_id>sqlc.arg(after_id) AND (sqlc.arg(search)::text='' OR c.character_id::text=sqlc.arg(search)
 OR EXISTS(SELECT 1 FROM eve_sync_targets t WHERE t.character_id=c.character_id AND t.display_name ILIKE '%'||sqlc.arg(search)||'%'))
AND (sqlc.arg(state_filter)::text='' OR
 CASE WHEN c.state='reauthorize' THEN 'reauthorize'
 WHEN o.consecutive_failures>0 THEN 'refresh_failed'
 WHEN o.access_expires_at IS NULL THEN 'unknown'
 WHEN o.access_expires_at<=now()+interval '1 minute' THEN 'refresh_due' ELSE 'valid' END=sqlc.arg(state_filter))
ORDER BY c.character_id LIMIT 31;

-- name: ListTokenEvents :many
SELECT id,generation,occurred_at,outcome,reason,duration_ms FROM eve_token_events WHERE character_id=$1 ORDER BY id DESC LIMIT 20;

-- name: TokenObservationExists :one
SELECT EXISTS(SELECT 1 FROM eve_credentials WHERE character_id=$1);

-- name: CleanupTokenEvents :exec
DELETE FROM eve_token_events WHERE id IN (SELECT id FROM eve_token_events WHERE occurred_at<now()-interval '30 days' ORDER BY occurred_at LIMIT 500);
