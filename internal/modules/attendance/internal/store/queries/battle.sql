-- name: SaveShip :one
INSERT INTO attendance_ships(event_id,character_id,request_key,ship_type_id,solar_system_id,joined_at,observed_at)
VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id;
-- name: GetShip :one
SELECT * FROM attendance_ships WHERE id=$1;
-- name: ListShips :many
SELECT * FROM attendance_ships WHERE event_id=$1 AND character_id=$2 ORDER BY id DESC LIMIT 21;
-- name: SaveFitting :exec
UPDATE attendance_ships SET fitting_state=$2,fitting=$3 WHERE id=$1;
-- name: SeedBattleTask :exec
INSERT INTO attendance_battle_tasks(event_id,character_id,account_id,owner_hash,kind,snapshot_id) VALUES($1,$2,$3,$4,$5,$6)
ON CONFLICT(event_id,character_id,kind,snapshot_id) DO NOTHING;
-- name: DueBattleTasks :many
SELECT id,kind FROM attendance_battle_tasks WHERE state IN ('pending','running') AND next_due_at<=now()
AND (lease_until IS NULL OR lease_until<now()) ORDER BY CASE WHEN kind='fitting' THEN 0 ELSE 1 END,next_due_at,id LIMIT 50;
-- name: ClaimBattleTask :one
UPDATE attendance_battle_tasks SET state='running',lease_until=now()+interval '90 seconds',fence=fence+1
WHERE id=$1 AND state IN ('pending','running') AND next_due_at<=now() AND (lease_until IS NULL OR lease_until<now()) RETURNING *;
-- name: LockBattleTask :one
SELECT * FROM attendance_battle_tasks WHERE id=$1 FOR UPDATE;
-- name: CompleteBattleTask :exec
UPDATE attendance_battle_tasks SET state=$2,reason=$3,next_due_at=$4,progress=$5,failures=$6,checked_at=now(),lease_until=NULL
WHERE id=$1 AND fence=$7 AND lease_until>now();
-- name: ListBattleTasks :many
SELECT DISTINCT ON (kind) id,kind,state,reason,checked_at,next_due_at FROM attendance_battle_tasks WHERE event_id=$1 AND character_id=$2 ORDER BY kind,id DESC;
-- name: SaveLoss :exec
INSERT INTO attendance_losses(event_id,character_id,killmail_id,occurred_at,ship_type_id,solar_system_id,items) VALUES($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT(event_id,killmail_id) DO NOTHING;
-- name: ListLosses :many
SELECT * FROM attendance_losses WHERE event_id=$1 AND character_id=$2 ORDER BY occurred_at DESC,killmail_id DESC LIMIT 101;
-- name: LockLoss :one
SELECT * FROM attendance_losses WHERE event_id=$1 AND killmail_id=$2 FOR UPDATE;
-- name: ReviewLoss :exec
UPDATE attendance_losses SET state=$3,version=version+1 WHERE event_id=$1 AND killmail_id=$2;
-- name: BattleEntry :one
SELECT * FROM attendance_entries WHERE event_id=$1 AND character_id=$2;
-- name: EntryShips :many
SELECT DISTINCT ON (character_id) character_id,ship_type_id,solar_system_id,observed_at FROM attendance_ships WHERE event_id=$1 ORDER BY character_id,observed_at DESC,id DESC;
-- name: EntryLossCounts :many
SELECT character_id,count(*)::int AS losses FROM attendance_losses WHERE event_id=$1 AND state='confirmed' GROUP BY character_id;
-- name: RetryBattleTasks :exec
UPDATE attendance_battle_tasks SET state='pending',reason='',failures=0,next_due_at=now(),fence=fence+1,lease_until=NULL
WHERE event_id=$1 AND character_id=$2 AND account_id=$3 AND owner_hash=$4
AND state IN ('ready','failed','blocked') AND kind='losses';
