-- name: GetCredentialForUpdate :one
SELECT * FROM eve_credentials WHERE character_id=$1 FOR UPDATE;
-- name: GetCredential :one
SELECT * FROM eve_credentials WHERE character_id=$1;
-- name: AdvanceGeneration :one
UPDATE eve_credentials SET grant_generation=grant_generation+1 WHERE character_id=$1 RETURNING grant_generation;
-- name: PersistRotation :exec
UPDATE eve_credentials SET sealed=$2,refresh_revision=refresh_revision+1,updated_at=now() WHERE character_id=$1;
-- name: SeedSyncTargets :exec
INSERT INTO eve_sync_targets(character_id,resource,generation,display_name,next_due_at,state,reason)
SELECT c.character_id,resource,c.grant_generation,sqlc.arg(display_name)::text,
 CASE WHEN resource IN ('profile','authorization') THEN c.next_sync_at ELSE now() END,
 CASE WHEN scope<>'' AND NOT scope=ANY(c.scopes) THEN 'blocked' ELSE 'idle' END,
 CASE WHEN scope<>'' AND NOT scope=ANY(c.scopes) THEN 'missing_scope' ELSE '' END
FROM eve_credentials c CROSS JOIN (VALUES ('profile',''),('authorization',''),('character_contracts','esi-contracts.read_character_contracts.v1'),('corporation_contracts','esi-contracts.read_corporation_contracts.v1')) r(resource,scope) WHERE c.character_id=sqlc.arg(character_id)
ON CONFLICT(character_id,resource) DO UPDATE SET generation=excluded.generation,display_name=excluded.display_name,
 state=excluded.state,reason=excluded.reason,active_job_id=NULL,lease_until=NULL,fence=eve_sync_targets.fence+1,failures=0,
 next_due_at=greatest(eve_sync_targets.next_due_at,now()),updated_at=now();
-- name: LockDueTargets :many
-- Give time-sensitive online observations the first slots in each bounded scan; the larger batch still leaves capacity for other resources.
SELECT * FROM eve_sync_targets WHERE state<>'blocked' AND ((resource NOT LIKE 'wallet_%' AND resource NOT LIKE 'corporation_wallet_%') OR sqlc.arg(wallet_enabled)::boolean) AND (resource<>'killmails' OR sqlc.arg(losses_enabled)::boolean) AND (resource<>'online' OR sqlc.arg(online_enabled)::boolean) AND (resource<>'fittings' OR sqlc.arg(fittings_enabled)::boolean) AND (resource<>'skills' OR sqlc.arg(fittings_enabled)::boolean OR sqlc.arg(skills_enabled)::boolean) AND (resource<>'skillqueue' OR sqlc.arg(skills_enabled)::boolean) AND next_due_at<=now() ORDER BY CASE WHEN resource='online' THEN 0 ELSE 1 END,next_due_at,id LIMIT 100 FOR UPDATE SKIP LOCKED;
-- name: GetSyncTarget :one
SELECT * FROM eve_sync_targets WHERE id=$1;
-- name: LockSyncTarget :one
SELECT * FROM eve_sync_targets WHERE id=$1 FOR UPDATE;
-- name: SetSyncJob :exec
UPDATE eve_sync_targets SET active_job_id=$2,state='queued',updated_at=now() WHERE id=$1;
-- name: ClaimSync :one
UPDATE eve_sync_targets SET state='running',fence=fence+1,last_attempt_at=now(),lease_until=now()+interval '90 seconds',updated_at=now()
WHERE id=$1 AND generation=$2 AND active_job_id=$3 AND (lease_until IS NULL OR lease_until<now()) RETURNING *;
-- name: BeginSyncRun :exec
INSERT INTO eve_sync_runs(target_id,job_id,fence) VALUES($1,$2,$3);
-- name: AbandonSyncRuns :exec
UPDATE eve_sync_runs SET outcome='interrupted',reason='worker_interrupted',finished_at=now() WHERE target_id=$1 AND finished_at IS NULL;
-- name: FinishSyncRun :exec
UPDATE eve_sync_runs SET finished_at=now(),outcome=$3,reason=$4,http_status=$5 WHERE target_id=$1 AND fence=$2;
-- name: FinishSyncTarget :execrows
UPDATE eve_sync_targets SET state=sqlc.arg(state),reason=sqlc.arg(reason),next_due_at=sqlc.arg(next_due_at),
 active_job_id=NULL,completed_job_id=sqlc.arg(job_id),lease_until=NULL,failures=sqlc.arg(failures),updated_at=now(),
 last_success_at=CASE WHEN sqlc.arg(success)::boolean THEN now() ELSE last_success_at END,
 valid_until=CASE WHEN sqlc.arg(success)::boolean THEN sqlc.narg(valid_until)::timestamptz ELSE valid_until END,
 content_updated_at=CASE WHEN sqlc.arg(success)::boolean THEN coalesce(sqlc.narg(content_updated_at)::timestamptz,content_updated_at,now()) ELSE content_updated_at END
WHERE id=sqlc.arg(id) AND fence=sqlc.arg(fence) AND generation=sqlc.arg(generation) AND lease_until>now();
-- name: DeferSyncTarget :execrows
UPDATE eve_sync_targets SET state='deferred',reason=$3,next_due_at=$4,lease_until=NULL,failures=$5,updated_at=now()
WHERE id=$1 AND fence=$2;
-- name: ResetSyncTarget :exec
UPDATE eve_sync_targets SET state='idle',reason='',active_job_id=NULL,lease_until=NULL,updated_at=now() WHERE id=$1;
-- name: BlockCharacterSync :exec
UPDATE eve_sync_targets SET state='blocked',reason=$2,active_job_id=NULL,lease_until=NULL,fence=fence+1,updated_at=now() WHERE character_id=$1 AND resource<>'profile';
-- name: ListCharacterSync :many
SELECT * FROM eve_sync_targets WHERE character_id=$1 ORDER BY resource;
-- name: ListSyncTargets :many
SELECT * FROM eve_sync_targets WHERE id>sqlc.arg(after_id) AND (sqlc.arg(state_filter)::text='' OR state=sqlc.arg(state_filter))
AND (sqlc.arg(search)::text='' OR display_name ILIKE '%'||sqlc.arg(search)||'%' OR character_id::text=sqlc.arg(search)) ORDER BY id LIMIT 31;
-- name: ListSyncRuns :many
SELECT * FROM eve_sync_runs WHERE target_id=$1 ORDER BY id DESC LIMIT 20;
-- name: SyncAudit :exec
INSERT INTO eve_sync_audit(user_id,character_id,target_id,action,outcome) VALUES($1,$2,$3,$4,$5);
-- name: SaveCharacterProfile :exec
INSERT INTO eve_character_profiles(character_id,name,corporation_id,alliance_id,checked_at,valid_until) VALUES($1,$2,$3,$4,now(),$5)
ON CONFLICT(character_id) DO UPDATE SET name=excluded.name,corporation_id=excluded.corporation_id,alliance_id=excluded.alliance_id,checked_at=excluded.checked_at,valid_until=excluded.valid_until;
-- name: SetSyncName :exec
UPDATE eve_sync_targets SET display_name=$2 WHERE character_id=$1;
-- name: ReadESICache :one
SELECT * FROM eve_esi_cache WHERE cache_key=$1;
-- name: DeleteESICache :exec
DELETE FROM eve_esi_cache WHERE cache_key=$1;
-- name: WriteESICache :exec
INSERT INTO eve_esi_cache(cache_key,character_id,generation,body,etag,last_modified,expires_at,content_updated_at,page_count) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT(cache_key) DO UPDATE SET body=excluded.body,etag=excluded.etag,last_modified=excluded.last_modified,expires_at=excluded.expires_at,content_updated_at=excluded.content_updated_at,page_count=excluded.page_count,updated_at=now();
-- name: DeletePrivateCache :exec
DELETE FROM eve_esi_cache WHERE character_id=sqlc.arg(character_id)::bigint;
-- name: EnsureESILimit :exec
INSERT INTO eve_esi_limits(limit_key) VALUES($1) ON CONFLICT DO NOTHING;
-- name: LockESILimit :one
SELECT * FROM eve_esi_limits WHERE limit_key=$1 FOR UPDATE;
-- name: ReserveESILimit :exec
UPDATE eve_esi_limits SET next_request_at=$2,updated_at=now() WHERE limit_key=$1;
-- name: BlockESILimit :exec
INSERT INTO eve_esi_limits(limit_key,blocked_until) VALUES($1,$2)
ON CONFLICT(limit_key) DO UPDATE SET blocked_until=greatest(eve_esi_limits.blocked_until,excluded.blocked_until),updated_at=now();
-- name: CleanupSyncCache :exec
DELETE FROM eve_esi_cache WHERE cache_key IN (SELECT cache_key FROM eve_esi_cache WHERE expires_at<now()-interval '1 day' ORDER BY expires_at LIMIT 500);
-- name: CacheUsage :one
SELECT count(*)::bigint AS entries,coalesce(sum(octet_length(body)),0)::bigint AS bytes FROM eve_esi_cache;
-- name: CleanupSyncRuns :exec
WITH expired AS (
 (SELECT id,finished_at FROM eve_sync_runs
  WHERE outcome='success' AND finished_at<now()-interval '14 days'
  ORDER BY finished_at,id LIMIT 500)
 UNION ALL
 (SELECT id,finished_at FROM eve_sync_runs
  WHERE outcome<>'success' AND finished_at<now()-interval '30 days'
  ORDER BY finished_at,id LIMIT 500)
), batch AS (
 SELECT id FROM expired ORDER BY finished_at,id LIMIT 500
)
DELETE FROM eve_sync_runs WHERE id IN (SELECT id FROM batch)
 AND ((outcome='success' AND finished_at<now()-interval '14 days')
   OR (outcome<>'success' AND finished_at<now()-interval '30 days'));
-- name: GetESIRoute :one
SELECT group_name FROM eve_esi_routes WHERE route_key=$1;
-- name: SetESIRoute :exec
INSERT INTO eve_esi_routes(route_key,group_name) VALUES($1,$2) ON CONFLICT(route_key) DO UPDATE SET group_name=excluded.group_name;
-- name: SaveESIBudget :exec
UPDATE eve_esi_limits SET remaining=$2,capacity=$3,window_seconds=$4,reset_at=$5,updated_at=now() WHERE limit_key=$1;
-- name: LockCacheQuota :exec
SELECT pg_advisory_xact_lock(743285110);
-- name: LockCacheCredential :one
SELECT grant_generation,state FROM eve_credentials WHERE character_id=$1 FOR SHARE;
-- name: SetRoleBarrier :exec
UPDATE eve_credentials SET roles_not_before=greatest(now()+interval '5 minutes',sqlc.arg(until_at)::timestamptz,
 (SELECT max(expires_at) FROM eve_esi_cache WHERE character_id=sqlc.arg(character_id)::bigint)) WHERE character_id=sqlc.arg(character_id);
