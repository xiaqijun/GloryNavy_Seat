-- name: StructureSources :many
SELECT c.character_id,c.grant_generation,c.owner_hash,s.corporation_id,s.corporation_name,s.alliance_id,s.ceo_id
FROM eve_credentials c JOIN eve_role_snapshots s USING(character_id)
WHERE c.state IN ('ready','retry') AND s.valid_until>now() AND s.owner_hash=c.owner_hash
AND (c.roles_not_before IS NULL OR c.roles_not_before<=now())
AND (c.character_id=s.ceo_id OR 'Director'=ANY(s.roles))
AND ('esi-corporations.read_structures.v1'=ANY(c.scopes) OR 'esi-corporations.read_starbases.v1'=ANY(c.scopes))
ORDER BY s.corporation_id,(c.character_id=s.ceo_id) DESC,c.character_id;
