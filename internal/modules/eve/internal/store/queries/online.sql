-- name: SeedOnlineTargets :exec
INSERT INTO eve_sync_targets(character_id,resource,generation,display_name,state,reason)
SELECT c.character_id,'online',c.grant_generation,coalesce(p.name,''),
 CASE WHEN c.state='reauthorize' OR NOT 'esi-location.read_online.v1'=ANY(c.scopes) THEN 'blocked' ELSE 'idle' END,
 CASE WHEN c.state='reauthorize' THEN 'reauthorize' WHEN NOT 'esi-location.read_online.v1'=ANY(c.scopes) THEN 'missing_scope' ELSE '' END
FROM eve_credentials c LEFT JOIN eve_character_profiles p ON p.character_id=c.character_id
ON CONFLICT(character_id,resource) DO UPDATE SET generation=excluded.generation,state=excluded.state,reason=excluded.reason,
 active_job_id=NULL,lease_until=NULL,fence=eve_sync_targets.fence+1,next_due_at=now(),failures=0
WHERE eve_sync_targets.generation<>excluded.generation OR eve_sync_targets.reason='module_disabled';
-- name: SaveOnlineSample :exec
INSERT INTO eve_online_samples(character_id,generation,observed_at,online,corporation_id) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING;
-- name: DeleteOnlineHistory :exec
DELETE FROM eve_online_samples WHERE character_id=$1;
-- name: CleanupOnlineSamples :exec
DELETE FROM eve_online_samples WHERE (character_id,observed_at) IN (SELECT character_id,observed_at FROM eve_online_samples WHERE observed_at<now()-interval '31 days' ORDER BY observed_at LIMIT 10000);
-- name: OnlineLatest :many
SELECT c.character_id,c.grant_generation,c.state,c.scopes,p.name,p.corporation_id,s.observed_at,s.online,coalesce(s.generation,0)::bigint AS generation,coalesce(s.corporation_id,0)::bigint AS observed_corporation_id
FROM eve_credentials c LEFT JOIN eve_character_profiles p ON p.character_id=c.character_id
LEFT JOIN LATERAL (SELECT * FROM eve_online_samples a WHERE a.character_id=c.character_id ORDER BY observed_at DESC LIMIT 1) s ON true
WHERE c.character_id=ANY(sqlc.arg(ids)::bigint[]) ORDER BY c.character_id;
-- name: OnlineIntervals :many
WITH observations AS (
 SELECT character_id,generation,observed_at,online,corporation_id,
 lag(corporation_id) OVER w AS previous_corporation,
 lag(observed_at) OVER w AS previous_at,lag(online) OVER w AS previous_online,lag(generation) OVER w AS previous_generation
 FROM eve_online_samples WHERE character_id=ANY(sqlc.arg(ids)::bigint[]) AND observed_at>=sqlc.arg(since)::timestamptz-interval '300 seconds' AND observed_at<=sqlc.arg(until_at)::timestamptz
 WINDOW w AS (PARTITION BY character_id ORDER BY observed_at)
), intervals AS (
 SELECT character_id,tstzrange(greatest(previous_at,sqlc.arg(since)::timestamptz),observed_at,'[)') AS span
 FROM observations WHERE online AND previous_online AND generation=previous_generation
 AND (sqlc.arg(corporation_id)::bigint=0 OR (corporation_id=sqlc.arg(corporation_id) AND previous_corporation=sqlc.arg(corporation_id)))
 AND observed_at>sqlc.arg(since)::timestamptz AND observed_at-previous_at<=interval '300 seconds'
), merged AS (SELECT character_id,range_agg(span) AS spans FROM intervals GROUP BY character_id)
SELECT character_id,lower(span)::timestamptz AS starts_at,upper(span)::timestamptz AS ends_at FROM merged CROSS JOIN LATERAL unnest(spans) span ORDER BY character_id,starts_at;
-- name: ActivityCorporations :many
SELECT DISTINCT corporation_id FROM eve_character_profiles ORDER BY corporation_id;
-- name: ActivityCharacters :many
SELECT character_id FROM eve_character_profiles WHERE corporation_id=$1 AND valid_until>now() ORDER BY character_id;

-- name: OnlineSampleCounts :many
SELECT character_id,to_char(observed_at AT TIME ZONE 'Asia/Shanghai','YYYY-MM-DD')::text AS day,count(*)::bigint AS samples
FROM eve_online_samples WHERE character_id=ANY(sqlc.arg(ids)::bigint[]) AND observed_at>=sqlc.arg(since)::timestamptz AND observed_at<=sqlc.arg(until_at)::timestamptz AND online IS NOT NULL AND (sqlc.arg(corporation_id)::bigint=0 OR corporation_id=sqlc.arg(corporation_id))
GROUP BY character_id,day ORDER BY day,character_id;
