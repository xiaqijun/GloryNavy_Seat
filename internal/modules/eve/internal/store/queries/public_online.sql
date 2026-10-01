-- name: PublicOnlineLatest :many
SELECT c.character_id,c.owner_hash,c.grant_generation,c.state,c.scopes,
 s.observed_at,s.online,coalesce(s.generation,0)::bigint AS generation
FROM eve_credentials c
JOIN eve_character_profiles p ON p.character_id=c.character_id AND p.corporation_id=sqlc.arg(corporation_id) AND p.valid_until>now()
LEFT JOIN LATERAL (SELECT a.* FROM eve_online_samples a WHERE a.character_id=c.character_id AND a.corporation_id=sqlc.arg(corporation_id) ORDER BY a.observed_at DESC LIMIT 1) s ON true
WHERE c.character_id=ANY(sqlc.arg(ids)::bigint[]);
