-- name: CreateEvent :one
INSERT INTO attendance_events(corporation_id,title,starts_at,created_by,request_key) VALUES($1,$2,$3,$4,$5)
ON CONFLICT(created_by,request_key) DO UPDATE SET request_key=excluded.request_key RETURNING *;
-- name: ListEvents :many
SELECT e.*, (SELECT count(DISTINCT a.account_id) FROM attendance_entries a WHERE a.event_id=e.id AND a.present)::int AS participants
FROM attendance_events e WHERE e.id<sqlc.arg(before_id)
AND (e.corporation_id=ANY(sqlc.arg(corporations)::bigint[]) OR EXISTS(SELECT 1 FROM attendance_entries a WHERE a.event_id=e.id AND a.account_id=sqlc.arg(account_id)::uuid AND a.present))
ORDER BY e.id DESC LIMIT 31;
-- name: ListPendingPAP :many
SELECT e.*, (SELECT count(DISTINCT a.account_id) FROM attendance_entries a WHERE a.event_id=e.id AND a.present AND a.account_id IS NOT NULL)::int AS participants
FROM attendance_events e
WHERE e.corporation_id=ANY(sqlc.arg(corporations)::bigint[])
  AND e.state='closed'
  AND NOT e.pap_issued
  AND EXISTS (SELECT 1 FROM attendance_entries a WHERE a.event_id=e.id AND a.present AND a.account_id IS NOT NULL)
ORDER BY e.ends_at DESC NULLS LAST, e.id DESC LIMIT 200;
-- name: GetEvent :one
SELECT * FROM attendance_events WHERE id=$1;
-- name: LockEvent :one
SELECT * FROM attendance_events WHERE id=$1 FOR UPDATE;
-- name: UpdateEvent :one
UPDATE attendance_events SET state=$2,version=version+1,ends_at=CASE WHEN $2='closed' THEN now() ELSE NULL END WHERE id=$1 RETURNING *;
-- name: ListEntries :many
SELECT * FROM attendance_entries WHERE event_id=$1 ORDER BY recorded_at,character_id;
-- name: SaveEntry :exec
INSERT INTO attendance_entries(event_id,character_id,account_id,character_name,source,present,recorded_at) VALUES($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT(event_id,character_id) DO UPDATE SET present=excluded.present,source=excluded.source,recorded_at=excluded.recorded_at
WHERE excluded.source='manual' OR attendance_entries.source<>'manual';
-- name: FindAudit :one
SELECT * FROM attendance_audit WHERE event_id=$1 AND request_key=$2;
-- name: Audit :exec
INSERT INTO attendance_audit(event_id,actor_id,action,request_key,payload) VALUES($1,$2,$3,$4,$5);
-- name: ListAudit :many
SELECT * FROM attendance_audit WHERE event_id=$1 AND id>sqlc.arg(after_id) ORDER BY id LIMIT 51;

-- name: EventCorporations :many
SELECT DISTINCT corporation_id FROM attendance_events ORDER BY corporation_id;
