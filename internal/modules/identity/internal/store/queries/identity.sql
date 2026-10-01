-- name: GetCharacter :one
SELECT * FROM identity_characters WHERE character_id = $1 FOR UPDATE;
-- name: SearchMembers :many
SELECT u.id, m.character_id, m.name,
 (SELECT count(*) FROM identity_characters c WHERE c.user_id=u.id)::int AS character_count
FROM identity_users u JOIN identity_characters m ON m.user_id=u.id AND m.character_id=u.main_character_id
WHERE u.id > sqlc.arg(after_id)::uuid AND
 (sqlc.arg(search)::text = '' OR u.id::text = sqlc.arg(search)::text OR EXISTS (
 SELECT 1 FROM identity_characters c WHERE c.user_id=u.id AND
 (strpos(lower(c.name),lower(sqlc.arg(search)::text)) > 0 OR c.character_id::text = sqlc.arg(search)::text)))
ORDER BY u.id LIMIT 26;
-- name: ActiveCharacters :many
SELECT character_id,owner_hash,name FROM identity_characters WHERE user_id=$1 AND status='active' ORDER BY character_id;
-- name: UserForCharacter :one
SELECT user_id FROM identity_characters WHERE character_id=$1 AND status='active';
-- name: CreateUser :one
INSERT INTO identity_users DEFAULT VALUES RETURNING id;
-- name: CreateCharacter :exec
INSERT INTO identity_characters (character_id, user_id, name, owner_hash) VALUES ($1, $2, $3, $4);
-- name: UpdateCharacter :exec
UPDATE identity_characters SET name = $2, verified_at = now() WHERE character_id = $1;
-- name: BlockCharacter :exec
UPDATE identity_characters SET status = 'blocked' WHERE character_id = $1;
-- name: RevokeCharacterSessions :exec
DELETE FROM identity_sessions WHERE character_id = $1;
-- name: InsertSession :exec
INSERT INTO identity_sessions (token_hash, user_id, character_id, csrf_token, expires_at) VALUES ($1, $2, $3, $4, $5);
-- name: GetSession :one
SELECT s.user_id, c.character_id, c.name, s.csrf_token, s.expires_at, m.character_id AS main_id, m.name AS main_name
FROM identity_sessions s JOIN identity_characters c ON c.character_id = s.character_id AND c.user_id = s.user_id
JOIN identity_users u ON u.id=s.user_id
JOIN identity_characters m ON m.character_id=u.main_character_id AND m.user_id=u.id
WHERE s.token_hash = $1 AND s.expires_at > now() AND c.status = 'active';

-- name: LockSession :one
SELECT s.user_id, s.character_id FROM identity_sessions s
JOIN identity_characters c ON c.character_id=s.character_id AND c.user_id=s.user_id
WHERE s.token_hash=$1 AND s.expires_at>now() AND c.status='active' FOR UPDATE OF s;
-- name: LockUser :one
SELECT * FROM identity_users WHERE id=$1 FOR UPDATE;
-- name: SetMain :exec
UPDATE identity_users SET main_character_id=$2 WHERE id=$1;
-- name: ListCharacters :many
SELECT c.character_id,c.name,c.status,(c.character_id=u.main_character_id)::boolean AS is_main
FROM identity_characters c JOIN identity_users u ON u.id=c.user_id
WHERE c.user_id=$1 ORDER BY (c.character_id=u.main_character_id) DESC,c.name,c.character_id;
-- name: DeleteCharacter :exec
DELETE FROM identity_characters WHERE character_id=$1 AND user_id=$2;
-- name: CharacterEvent :exec
INSERT INTO identity_character_events(user_id,character_id,action) VALUES($1,$2,$3);
-- name: DeleteSession :exec
DELETE FROM identity_sessions WHERE token_hash = $1;
-- name: PruneSessions :exec
DELETE FROM identity_sessions WHERE token_hash IN (
  SELECT token_hash FROM identity_sessions WHERE expires_at <= now() LIMIT 1000
);
-- name: LimitUserSessions :exec
DELETE FROM identity_sessions WHERE token_hash IN (
  SELECT s.token_hash FROM identity_sessions s WHERE s.user_id = $1 ORDER BY s.created_at DESC, s.token_hash OFFSET 4
);

-- name: BoundCharacters :many
SELECT character_id,user_id,name,owner_hash FROM identity_characters WHERE character_id=ANY($1::bigint[]) AND status='active' ORDER BY character_id;
