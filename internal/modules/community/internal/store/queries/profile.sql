-- name: EnsureProfile :exec
INSERT INTO community_profiles(user_id) VALUES($1) ON CONFLICT(user_id) DO NOTHING;
-- name: LockProfile :one
SELECT * FROM community_profiles WHERE user_id=$1 FOR UPDATE;
-- name: GetProfile :one
SELECT p.*,
 EXISTS(SELECT 1 FROM community_confirmations c WHERE c.user_id=p.user_id AND c.platform='qq' AND c.field_version=p.qq_version AND c.invalidated_at IS NULL) AS qq_confirmed,
 EXISTS(SELECT 1 FROM community_confirmations c WHERE c.user_id=p.user_id AND c.platform='kook' AND c.field_version=p.kook_version AND c.invalidated_at IS NULL) AS kook_confirmed
FROM community_profiles p WHERE p.user_id=$1;
-- name: UpdateProfile :exec
UPDATE community_profiles SET qq_number=$2,kook_name=$3,
 qq_version=qq_version+CASE WHEN qq_number<>$2 THEN 1 ELSE 0 END,
 kook_version=kook_version+CASE WHEN kook_name<>$3 THEN 1 ELSE 0 END,
 version=version+1,updated_at=now() WHERE user_id=$1;
-- name: InvalidateConfirmations :exec
UPDATE community_confirmations SET invalidated_at=now() WHERE user_id=$1 AND platform=ANY($2::text[]) AND invalidated_at IS NULL;
-- name: RecordProfileChange :exec
INSERT INTO community_profile_events(user_id,profile_version,changed_platforms) VALUES($1,$2,$3);
