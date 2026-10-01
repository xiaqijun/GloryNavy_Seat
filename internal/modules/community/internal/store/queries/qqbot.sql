-- name: FindQQProfile :one
SELECT p.*,
 EXISTS(SELECT 1 FROM community_confirmations c WHERE c.user_id=p.user_id AND c.platform='qq' AND c.field_version=p.qq_version AND c.invalidated_at IS NULL) AS qq_confirmed,
 EXISTS(SELECT 1 FROM community_confirmations c WHERE c.user_id=p.user_id AND c.platform='kook' AND c.field_version=p.kook_version AND c.invalidated_at IS NULL) AS kook_confirmed
FROM community_profiles p
WHERE p.qq_number=$1
FOR UPDATE;

-- name: GetBotEvent :one
SELECT source, event_id, payload_hash, received_at
FROM community_bot_events
WHERE source=$1 AND event_id=$2;

-- name: InsertBotEvent :exec
INSERT INTO community_bot_events(source,event_id,payload_hash)
VALUES($1,$2,$3)
ON CONFLICT(source,event_id) DO NOTHING;

-- name: InvalidateQQConfirmations :exec
UPDATE community_confirmations
SET invalidated_at=now()
WHERE user_id=$1 AND platform='qq' AND invalidated_at IS NULL;

-- name: InsertConfirmation :exec
INSERT INTO community_confirmations(user_id,platform,field_version,source,actor,event_id)
VALUES($1,'qq',$2,$3,$4,$5);
