-- name: StructureSources :many
SELECT c.character_id,c.grant_generation,c.owner_hash,s.corporation_id,s.corporation_name,s.alliance_id,s.ceo_id
FROM eve_credentials c JOIN eve_role_snapshots s USING(character_id)
WHERE c.state IN ('ready','retry') AND s.valid_until>now() AND s.owner_hash=c.owner_hash
AND (c.roles_not_before IS NULL OR c.roles_not_before<=now())
AND (c.character_id=s.ceo_id OR 'Director'=ANY(s.roles))
AND ('esi-corporations.read_structures.v1'=ANY(c.scopes) OR 'esi-corporations.read_starbases.v1'=ANY(c.scopes))
ORDER BY s.corporation_id,(c.character_id=s.ceo_id) DESC,c.character_id;

-- name: ReadStructureSnapshots :many
SELECT ss.character_id,ss.generation,ss.owner_hash,ss.corporation_id,ss.corporation_name,
       s.alliance_id,s.ceo_id,ss.observed_at,ss.valid_until,ss.payload
FROM eve_structure_snapshots ss
JOIN eve_credentials c ON c.character_id=ss.character_id
JOIN eve_role_snapshots s ON s.character_id=ss.character_id
WHERE c.state IN ('ready','retry')
  AND s.valid_until>now()
  AND s.owner_hash=c.owner_hash
  AND ss.generation=c.grant_generation
  AND ss.owner_hash=c.owner_hash
  AND ss.valid_until>now()
  AND (c.roles_not_before IS NULL OR c.roles_not_before<=now())
  AND (c.character_id=s.ceo_id OR 'Director'=ANY(s.roles))
  AND ('esi-corporations.read_structures.v1'=ANY(c.scopes) OR 'esi-corporations.read_starbases.v1'=ANY(c.scopes))
ORDER BY ss.corporation_id,(ss.character_id=s.ceo_id) DESC,ss.character_id;

-- name: SaveStructureSnapshot :exec
INSERT INTO eve_structure_snapshots(character_id,generation,owner_hash,corporation_id,corporation_name,observed_at,valid_until,payload)
VALUES($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT(character_id) DO UPDATE SET generation=excluded.generation,owner_hash=excluded.owner_hash,
 corporation_id=excluded.corporation_id,corporation_name=excluded.corporation_name,observed_at=excluded.observed_at,
 valid_until=excluded.valid_until,payload=excluded.payload,updated_at=now();
