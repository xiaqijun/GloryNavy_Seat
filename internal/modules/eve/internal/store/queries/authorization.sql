-- name: SaveCredential :exec
INSERT INTO eve_credentials(character_id, owner_hash, sealed, scopes) VALUES ($1,$2,$3,$4)
ON CONFLICT(character_id) DO UPDATE SET owner_hash=excluded.owner_hash, sealed=excluded.sealed,
state=CASE WHEN eve_credentials.owner_hash=excluded.owner_hash AND eve_credentials.state IN ('ready','retry') THEN eve_credentials.state ELSE 'pending' END,
next_sync_at=CASE WHEN eve_credentials.owner_hash=excluded.owner_hash AND eve_credentials.state <> 'reauthorize' THEN greatest(eve_credentials.next_sync_at,now()) ELSE now() END,
updated_at=now(), scopes=excluded.scopes;
-- name: InvalidateSnapshot :exec
DELETE FROM eve_role_snapshots WHERE character_id=$1;
-- name: DeleteCredential :exec
DELETE FROM eve_credentials WHERE character_id=$1;
-- name: RevokeCredential :exec
UPDATE eve_credentials SET state='reauthorize', sealed=''::bytea, scopes='{}' WHERE character_id=$1;
-- name: NextCredential :one
SELECT * FROM eve_credentials WHERE state <> 'reauthorize' AND next_sync_at <= now()
ORDER BY next_sync_at, character_id LIMIT 1 FOR UPDATE SKIP LOCKED;
-- name: UpdateCredential :exec
UPDATE eve_credentials SET sealed=$2, state=$3, next_sync_at=$4, scopes=$5, updated_at=now() WHERE character_id=$1;
-- name: SaveSnapshot :exec
INSERT INTO eve_role_snapshots(character_id,owner_hash,corporation_id,corporation_name,alliance_id,ceo_id,roles,roles_at_hq,roles_at_base,roles_at_other,synced_at,valid_until)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
ON CONFLICT(character_id) DO UPDATE SET owner_hash=excluded.owner_hash, corporation_id=excluded.corporation_id,
corporation_name=excluded.corporation_name, alliance_id=excluded.alliance_id, ceo_id=excluded.ceo_id,
roles=excluded.roles, roles_at_hq=excluded.roles_at_hq, roles_at_base=excluded.roles_at_base,
roles_at_other=excluded.roles_at_other, synced_at=excluded.synced_at, valid_until=excluded.valid_until;
-- name: GetAuthorization :one
SELECT c.state,c.owner_hash,c.scopes,s.corporation_id,s.corporation_name,s.alliance_id,s.ceo_id,s.roles,s.roles_at_hq,s.roles_at_base,s.roles_at_other,s.synced_at,s.valid_until
FROM eve_credentials c LEFT JOIN eve_role_snapshots s USING(character_id) WHERE c.character_id=$1;
-- name: CorporationSnapshot :one
SELECT corporation_id,corporation_name,alliance_id,ceo_id FROM eve_role_snapshots
WHERE corporation_id=$1 AND valid_until>now() ORDER BY synced_at DESC LIMIT 1;
