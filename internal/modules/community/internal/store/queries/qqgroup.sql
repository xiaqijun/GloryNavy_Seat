-- name: InsertQQGroupApplication :one
INSERT INTO community_qq_group_applications(user_id,group_openid,qq_number,qq_version,code_hash,status,expires_at)
VALUES($1,$2,$3,$4,$5,'pending',$6)
RETURNING id,group_openid,qq_number,qq_version,status,expires_at;

-- name: LockActiveQQGroupApplication :one
SELECT id,user_id,group_openid,qq_number,qq_version,code_hash,status,member_openid,join_request_id,
 failure_reason,created_at,expires_at,approved_at,bound_at,updated_at
FROM community_qq_group_applications
WHERE user_id=$1 AND group_openid=$2 AND status IN ('pending','approving','approved','bound')
ORDER BY created_at DESC LIMIT 1 FOR UPDATE;

-- name: FindQQGroupApplicationByCode :one
SELECT id,user_id,group_openid,qq_number,qq_version,code_hash,status,member_openid,join_request_id,
 failure_reason,created_at,expires_at,approved_at,bound_at,updated_at
FROM community_qq_group_applications
WHERE code_hash=$1 AND status='pending' AND expires_at>now()
FOR UPDATE;

-- name: LatestQQGroupApplication :one
SELECT id,user_id,group_openid,qq_number,qq_version,code_hash,status,member_openid,join_request_id,
 failure_reason,created_at,expires_at,approved_at,bound_at,updated_at
FROM community_qq_group_applications
WHERE user_id=$1
ORDER BY created_at DESC LIMIT 1;

-- name: LockQQGroupApplication :one
SELECT id,user_id,group_openid,qq_number,qq_version,code_hash,status,member_openid,join_request_id,
 failure_reason,created_at,expires_at,approved_at,bound_at,updated_at
FROM community_qq_group_applications WHERE id=$1 FOR UPDATE;

-- name: MarkQQGroupApplicationApproving :exec
UPDATE community_qq_group_applications
SET status='approving',member_openid=$2,join_request_id=$3,updated_at=now()
WHERE id=$1 AND status='pending';

-- name: MarkQQGroupApplicationApproved :exec
UPDATE community_qq_group_applications
SET status='approved',approved_at=now(),failure_reason=NULL,updated_at=now()
WHERE id=$1 AND status='approving';

-- name: MarkQQGroupApplicationFailed :exec
UPDATE community_qq_group_applications
SET status='failed',failure_reason=$2,updated_at=now()
WHERE id=$1 AND status IN ('approving','pending');

-- name: MarkQQGroupApplicationRejected :exec
UPDATE community_qq_group_applications
SET status='rejected',failure_reason=$2,updated_at=now()
WHERE id=$1 AND status IN ('pending','approving');

-- name: ListQQGroupApplications :many
SELECT id,user_id,group_openid,qq_number,qq_version,status,member_openid,join_request_id,failure_reason,
 created_at,expires_at,approved_at,bound_at,updated_at
FROM community_qq_group_applications
WHERE ($1::text='' OR group_openid=$1) AND ($2::text='' OR status=$2)
ORDER BY created_at DESC LIMIT $3;

-- name: FindQQGroupApplicationByMember :one
SELECT id,user_id,group_openid,qq_number,qq_version,code_hash,status,member_openid,join_request_id,
 failure_reason,created_at,expires_at,approved_at,bound_at,updated_at
FROM community_qq_group_applications
WHERE group_openid=$1 AND member_openid=$2 AND status='approved'
ORDER BY updated_at DESC LIMIT 1 FOR UPDATE;

-- name: InsertQQGroupBinding :exec
INSERT INTO community_qq_group_bindings(user_id,group_openid,member_openid,qq_number,application_id)
VALUES($1,$2,$3,$4,$5)
ON CONFLICT(group_openid,member_openid) DO UPDATE SET user_id=EXCLUDED.user_id,qq_number=EXCLUDED.qq_number,application_id=EXCLUDED.application_id,bound_at=now();

-- name: MarkQQGroupApplicationBound :exec
UPDATE community_qq_group_applications
SET status='bound',bound_at=now(),updated_at=now()
WHERE id=$1 AND status='approved';

-- name: ExpireQQGroupApplications :exec
UPDATE community_qq_group_applications SET status='expired',updated_at=now()
WHERE status IN ('pending','approving') AND expires_at<=now();

-- name: QQGroupSettingsInitialized :one
SELECT EXISTS(SELECT 1 FROM community_qq_group_settings_meta WHERE id=true);

-- name: ListQQGroupSettings :many
SELECT group_openid,label,enabled,created_at,updated_at
FROM community_qq_group_settings
ORDER BY created_at ASC, group_openid ASC;

-- name: ReplaceQQGroupSettings :exec
DELETE FROM community_qq_group_settings;

-- name: InsertQQGroupSetting :exec
INSERT INTO community_qq_group_settings(group_openid,label,enabled)
VALUES($1,$2,$3);

-- name: MarkQQGroupSettingsInitialized :exec
INSERT INTO community_qq_group_settings_meta(id,updated_at) VALUES(true,now())
ON CONFLICT(id) DO UPDATE SET updated_at=EXCLUDED.updated_at;

-- name: QQBotSettingsInitialized :one
SELECT EXISTS(SELECT 1 FROM community_qq_bot_settings WHERE id=true);

-- name: GetQQBotSettings :one
SELECT app_id,api_base,secret_ciphertext,created_at,updated_at
FROM community_qq_bot_settings
WHERE id=true;

-- name: UpsertQQBotSettings :exec
INSERT INTO community_qq_bot_settings(id,app_id,api_base,secret_ciphertext,updated_at)
VALUES(true,$1,$2,$3,now())
ON CONFLICT(id) DO UPDATE SET app_id=EXCLUDED.app_id,api_base=EXCLUDED.api_base,
 secret_ciphertext=EXCLUDED.secret_ciphertext,updated_at=EXCLUDED.updated_at;
