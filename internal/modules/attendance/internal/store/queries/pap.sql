-- name: PAPAwards :many
SELECT * FROM attendance_pap_awards WHERE event_id=$1 ORDER BY character_id;
-- name: SavePAPAward :exec
INSERT INTO attendance_pap_awards(event_id,character_id,account_id,character_name,points) VALUES($1,$2,$3,$4,$5)
ON CONFLICT(event_id,character_id) DO UPDATE SET points=excluded.points,updated_at=now();
-- name: AddPAPLedger :exec
INSERT INTO attendance_pap_ledger(event_id,character_id,account_id,actor_id,request_key,delta,balance,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8);
-- name: SetEventPAP :exec
UPDATE attendance_events SET pap_points=$2,pap_issued=$3,version=version+1 WHERE id=$1;
-- name: PAPReport :many
SELECT a.*,e.title,e.starts_at,e.corporation_id
FROM attendance_pap_awards a JOIN attendance_events e ON e.id=a.event_id
WHERE a.points>0 AND e.starts_at>=sqlc.arg(since) AND e.starts_at<sqlc.arg(until)
AND ((sqlc.arg(corporation_id)::bigint>0 AND e.corporation_id=sqlc.arg(corporation_id)) OR (sqlc.arg(corporation_id)=0 AND a.account_id=sqlc.arg(account_id)::uuid))
ORDER BY e.starts_at DESC,a.event_id DESC,a.character_id LIMIT 51 OFFSET sqlc.arg(row_offset);
-- name: PAPTotals :one
SELECT coalesce(sum(a.points),0)::bigint AS points,count(DISTINCT a.event_id)::bigint AS events,count(*)::bigint AS participations
FROM attendance_pap_awards a JOIN attendance_events e ON e.id=a.event_id
WHERE a.points>0 AND e.starts_at>=sqlc.arg(since) AND e.starts_at<sqlc.arg(until)
AND ((sqlc.arg(corporation_id)::bigint>0 AND e.corporation_id=sqlc.arg(corporation_id)) OR (sqlc.arg(corporation_id)=0 AND a.account_id=sqlc.arg(account_id)::uuid));
-- name: PAPLedger :many
SELECT l.*,a.character_name FROM attendance_pap_ledger l JOIN attendance_pap_awards a USING(event_id,character_id)
WHERE l.event_id=$1 AND l.id>sqlc.arg(after_id) AND (sqlc.arg(manage)::boolean OR l.account_id=sqlc.arg(account_id)::uuid)
ORDER BY l.id LIMIT 51;
