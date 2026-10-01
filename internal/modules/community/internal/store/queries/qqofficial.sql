-- name: InvalidateQQChallenges :exec
UPDATE community_qq_link_challenges SET used_at=COALESCE(used_at,now()) WHERE user_id=$1 AND used_at IS NULL;

-- name: InsertQQChallenge :exec
INSERT INTO community_qq_link_challenges(user_id,code_hash,qq_version,expires_at) VALUES($1,$2,$3,$4);

-- name: LockQQChallenge :one
SELECT id,user_id,qq_version,expires_at,used_at FROM community_qq_link_challenges WHERE code_hash=$1 FOR UPDATE;

-- name: MarkQQChallengeUsed :exec
UPDATE community_qq_link_challenges SET used_at=now() WHERE id=$1 AND used_at IS NULL;

-- name: UpsertQQBinding :exec
INSERT INTO community_qq_bindings(user_id,openid,qq_version) VALUES($1,$2,$3)
ON CONFLICT(user_id) DO UPDATE SET openid=EXCLUDED.openid,qq_version=EXCLUDED.qq_version,bound_at=now();

-- name: FindQQBinding :one
SELECT user_id,openid,qq_version FROM community_qq_bindings WHERE openid=$1;
