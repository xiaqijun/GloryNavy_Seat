-- name: OneBotSelected :one
SELECT EXISTS(SELECT 1 FROM community_onebot_settings);
-- name: GetOneBotSettings :one
SELECT * FROM community_onebot_settings WHERE singleton=true;
-- name: LockOneBotSettings :one
SELECT * FROM community_onebot_settings WHERE singleton=true FOR SHARE;
-- name: LockOneBotConfigWrite :exec
SELECT pg_advisory_xact_lock(hashtextextended('community.onebot.config',0));
-- name: SaveOneBotSettings :exec
INSERT INTO community_onebot_settings(singleton,enabled,api_base,bot_qq,token_ciphertext,secret_ciphertext)
VALUES(true,$1,$2,$3,$4,$5)
ON CONFLICT(singleton) DO UPDATE SET enabled=excluded.enabled,api_base=excluded.api_base,bot_qq=excluded.bot_qq,
token_ciphertext=excluded.token_ciphertext,secret_ciphertext=excluded.secret_ciphertext,revision=community_onebot_settings.revision+1,updated_at=now();
-- name: ListOneBotGroups :many
SELECT group_id,name,notify_approvals FROM community_onebot_groups ORDER BY name,group_id;
-- name: ClearOneBotGroups :exec
DELETE FROM community_onebot_groups;
-- name: InsertOneBotGroup :exec
INSERT INTO community_onebot_groups(group_id,name,notify_approvals) VALUES($1,$2,$3);
-- name: OneBotGroupAllowed :one
SELECT EXISTS(SELECT 1 FROM community_onebot_groups WHERE group_id=$1);
-- name: LockOneBotEvent :exec
SELECT pg_advisory_xact_lock(hashtextextended('community.onebot.event:' || $1::text,0));
-- name: LockOneBotQQ :exec
SELECT pg_advisory_xact_lock(hashtextextended('community.onebot.qq:' || $1::text,0));
-- name: FindOneBotBinding :one
SELECT * FROM community_onebot_bindings WHERE qq_number=$1;
-- name: GetOneBotBinding :one
SELECT b.* FROM community_onebot_bindings b JOIN community_profiles p ON p.user_id=b.user_id
WHERE b.user_id=$1 AND b.qq_number=p.qq_number AND b.qq_version=p.qq_version
AND EXISTS(SELECT 1 FROM community_confirmations c WHERE c.user_id=b.user_id AND c.platform='qq' AND c.field_version=b.qq_version AND c.source='onebot' AND c.invalidated_at IS NULL);
-- name: SaveOneBotBinding :exec
INSERT INTO community_onebot_bindings(user_id,qq_number,qq_version) VALUES($1,$2,$3)
ON CONFLICT(user_id) DO UPDATE SET qq_number=excluded.qq_number,qq_version=excluded.qq_version,bound_at=now();
-- name: CountOneBotBindings :one
SELECT count(*) FROM community_onebot_bindings WHERE user_id=$1;
-- name: DeleteOneBotBindings :exec
DELETE FROM community_onebot_bindings WHERE user_id=$1;
-- name: OneBotChallengeOwner :one
SELECT user_id FROM community_qq_link_challenges WHERE code_hash=$1;
