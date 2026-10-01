-- name: SeedFittingTargets :exec
INSERT INTO eve_sync_targets(character_id,resource,generation,display_name,state,reason)
SELECT c.character_id,r.resource,c.grant_generation,coalesce(p.name,''),
 CASE WHEN c.state='reauthorize' OR NOT r.scope=ANY(c.scopes) THEN 'blocked' ELSE 'idle' END,
 CASE WHEN c.state='reauthorize' THEN 'reauthorize' WHEN NOT r.scope=ANY(c.scopes) THEN 'missing_scope' ELSE '' END
FROM eve_credentials c LEFT JOIN eve_character_profiles p ON p.character_id=c.character_id CROSS JOIN (VALUES ('fittings','esi-fittings.read_fittings.v1'),('skills','esi-skills.read_skills.v1'),('skillqueue','esi-skills.read_skillqueue.v1')) r(resource,scope)
WHERE (r.resource='fittings' AND sqlc.arg(fittings_enabled)::boolean) OR (r.resource='skills' AND (sqlc.arg(fittings_enabled)::boolean OR sqlc.arg(skills_enabled)::boolean)) OR (r.resource='skillqueue' AND sqlc.arg(skills_enabled)::boolean)
ON CONFLICT(character_id,resource) DO UPDATE SET generation=excluded.generation,state=excluded.state,reason=excluded.reason,active_job_id=NULL,lease_until=NULL,fence=eve_sync_targets.fence+1,next_due_at=now(),failures=0
WHERE eve_sync_targets.generation<>excluded.generation OR eve_sync_targets.reason='module_disabled';
-- name: SaveFittingSnapshot :exec
INSERT INTO eve_fitting_snapshots(character_id,resource,generation,observed_at,payload) VALUES($1,$2,$3,$4,$5)
ON CONFLICT(character_id,resource) DO UPDATE SET generation=excluded.generation,observed_at=excluded.observed_at,payload=excluded.payload;
-- name: ReadFittingSnapshot :one
SELECT s.* FROM eve_fitting_snapshots s JOIN eve_credentials c USING(character_id)
WHERE s.character_id=$1 AND s.resource=$2 AND s.generation=c.grant_generation AND c.state<>'reauthorize' AND (CASE WHEN s.resource='fittings' THEN 'esi-fittings.read_fittings.v1' WHEN s.resource='skillqueue' THEN 'esi-skills.read_skillqueue.v1' ELSE 'esi-skills.read_skills.v1' END)=ANY(c.scopes);

-- name: RetainSkillSnapshots :exec
-- SaveTx has locked the credential and verified the same owner. Carry only the
-- immediately preceding grant's observations with continuously granted scopes.
WITH retained AS (
 UPDATE eve_fitting_snapshots s SET generation=c.grant_generation
 FROM eve_credentials c
 WHERE s.character_id=sqlc.arg(character_id) AND c.character_id=s.character_id
 AND s.generation=sqlc.arg(previous_generation) AND c.state<>'reauthorize'
 AND s.resource IN ('skills','skillqueue')
 AND (CASE WHEN s.resource='skills' THEN 'esi-skills.read_skills.v1' ELSE 'esi-skills.read_skillqueue.v1' END)=ANY(c.scopes)
 AND (CASE WHEN s.resource='skills' THEN 'esi-skills.read_skills.v1' ELSE 'esi-skills.read_skillqueue.v1' END)=ANY(sqlc.arg(previous_scopes)::text[])
 RETURNING s.character_id,s.resource
)
UPDATE eve_sync_targets t SET valid_until=NULL
FROM retained r WHERE t.character_id=r.character_id AND t.resource=r.resource;
