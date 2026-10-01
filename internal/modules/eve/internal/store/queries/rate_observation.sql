-- name: ObserveESIBucket :one
INSERT INTO eve_esi_buckets(limit_key,group_name,character_id,header_at,capacity,window_seconds,remaining,retry_at)
VALUES(sqlc.arg(limit_key),sqlc.arg(group_name),sqlc.arg(character_id),sqlc.narg(header_at),sqlc.narg(capacity),sqlc.narg(window_seconds),sqlc.narg(remaining),sqlc.narg(retry_at))
ON CONFLICT(limit_key) DO UPDATE SET updated_at=clock_timestamp(),
 header_at=CASE WHEN excluded.header_at>=eve_esi_buckets.header_at OR eve_esi_buckets.header_at IS NULL THEN coalesce(excluded.header_at,eve_esi_buckets.header_at) ELSE eve_esi_buckets.header_at END,
 capacity=CASE WHEN excluded.header_at>=eve_esi_buckets.header_at OR eve_esi_buckets.header_at IS NULL THEN coalesce(excluded.capacity,eve_esi_buckets.capacity) ELSE eve_esi_buckets.capacity END,
 window_seconds=CASE WHEN excluded.header_at>=eve_esi_buckets.header_at OR eve_esi_buckets.header_at IS NULL THEN coalesce(excluded.window_seconds,eve_esi_buckets.window_seconds) ELSE eve_esi_buckets.window_seconds END,
 remaining=CASE WHEN excluded.header_at>=eve_esi_buckets.header_at OR eve_esi_buckets.header_at IS NULL THEN excluded.remaining ELSE eve_esi_buckets.remaining END,
 retry_at=greatest(eve_esi_buckets.retry_at,excluded.retry_at)
RETURNING id;

-- name: ObserveESIRouteUsage :exec
INSERT INTO eve_esi_route_usage(bucket_id,route,network_requests,cache_hits,local_waits,upstream_limits,used_tokens,measured_responses,unmeasured_requests,last_status,last_used,last_response_at)
VALUES(sqlc.arg(bucket_id),sqlc.arg(route),sqlc.arg(network_requests),sqlc.arg(cache_hits),sqlc.arg(local_waits),sqlc.arg(upstream_limits),sqlc.arg(used_tokens),sqlc.arg(measured_responses),sqlc.arg(unmeasured_requests),sqlc.arg(last_status),sqlc.narg(last_used),sqlc.narg(last_response_at))
ON CONFLICT(bucket_id,route) DO UPDATE SET
 network_requests=eve_esi_route_usage.network_requests+excluded.network_requests,
 cache_hits=eve_esi_route_usage.cache_hits+excluded.cache_hits,
 local_waits=eve_esi_route_usage.local_waits+excluded.local_waits,
 upstream_limits=eve_esi_route_usage.upstream_limits+excluded.upstream_limits,
 used_tokens=eve_esi_route_usage.used_tokens+excluded.used_tokens,
 measured_responses=eve_esi_route_usage.measured_responses+excluded.measured_responses,
 unmeasured_requests=eve_esi_route_usage.unmeasured_requests+excluded.unmeasured_requests,
 last_status=CASE WHEN excluded.last_response_at>=eve_esi_route_usage.last_response_at OR eve_esi_route_usage.last_response_at IS NULL THEN excluded.last_status ELSE eve_esi_route_usage.last_status END,
 last_used=CASE WHEN excluded.last_response_at>=eve_esi_route_usage.last_response_at OR eve_esi_route_usage.last_response_at IS NULL THEN excluded.last_used ELSE eve_esi_route_usage.last_used END,
 last_response_at=greatest(eve_esi_route_usage.last_response_at,excluded.last_response_at),updated_at=clock_timestamp();

-- name: ListESIBuckets :many
SELECT b.*,coalesce((SELECT display_name FROM eve_sync_targets t WHERE t.character_id=b.character_id ORDER BY t.id LIMIT 1),'')::text AS display_name,
 CASE WHEN l.capacity>0 THEN greatest(0,l.capacity-(SELECT coalesce(sum(c.amount),0) FROM eve_esi_charges c WHERE c.limit_key=b.limit_key AND c.expires_at>clock_timestamp())) ELSE -1 END::bigint AS local_remaining,
 l.capacity AS local_capacity,
 (SELECT min(c.expires_at) FROM eve_esi_charges c WHERE c.limit_key=b.limit_key AND c.amount>0 AND c.expires_at>clock_timestamp())::timestamptz AS local_reset_at,l.blocked_until,
 (SELECT blocked_until FROM eve_esi_limits WHERE limit_key='egress')::timestamptz AS egress_blocked_until,
 coalesce((SELECT sum(u.used_tokens) FROM eve_esi_route_usage u WHERE u.bucket_id=b.id),0)::bigint AS used_tokens,
 coalesce((SELECT sum(u.network_requests) FROM eve_esi_route_usage u WHERE u.bucket_id=b.id),0)::bigint AS network_requests,
 coalesce((SELECT sum(u.unmeasured_requests) FROM eve_esi_route_usage u WHERE u.bucket_id=b.id),0)::bigint AS unmeasured_requests
FROM eve_esi_buckets b LEFT JOIN eve_esi_limits l USING(limit_key)
WHERE b.group_name<>'' AND b.id>sqlc.arg(after_id) AND (sqlc.arg(search)::text='' OR b.group_name ILIKE '%'||sqlc.arg(search)||'%' OR b.character_id::text=sqlc.arg(search)
 OR EXISTS(SELECT 1 FROM eve_sync_targets t WHERE t.character_id=b.character_id AND t.display_name ILIKE '%'||sqlc.arg(search)||'%'))
ORDER BY b.id LIMIT 31;

-- name: ESIBucketExists :one
SELECT EXISTS(SELECT 1 FROM eve_esi_buckets WHERE id=$1 AND group_name<>'');

-- name: ListESIRouteUsage :many
SELECT * FROM eve_esi_route_usage WHERE bucket_id=sqlc.arg(bucket_id) AND route>sqlc.arg(after_route)::text ORDER BY route LIMIT 31;

-- name: CleanupESIBuckets :exec
DELETE FROM eve_esi_buckets WHERE id IN (SELECT id FROM eve_esi_buckets WHERE updated_at<now()-interval '30 days' ORDER BY updated_at LIMIT 100);
