-- +goose Up
-- eve: operational rate-limit metadata, separate from SSO credentials.
CREATE TABLE eve_esi_buckets (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 limit_key text NOT NULL UNIQUE,
 group_name text NOT NULL,
 character_id bigint NOT NULL DEFAULT 0,
 observed_since timestamptz NOT NULL DEFAULT clock_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 header_at timestamptz,
 capacity bigint,
 window_seconds bigint,
 remaining bigint,
 retry_at timestamptz
);
CREATE TABLE eve_esi_route_usage (
 bucket_id bigint NOT NULL REFERENCES eve_esi_buckets(id) ON DELETE CASCADE,
 route text NOT NULL,
 network_requests bigint NOT NULL DEFAULT 0,
 cache_hits bigint NOT NULL DEFAULT 0,
 local_waits bigint NOT NULL DEFAULT 0,
 upstream_limits bigint NOT NULL DEFAULT 0,
 used_tokens bigint NOT NULL DEFAULT 0,
 measured_responses bigint NOT NULL DEFAULT 0,
 unmeasured_requests bigint NOT NULL DEFAULT 0,
 last_status integer NOT NULL DEFAULT 0,
 last_used bigint,
 last_response_at timestamptz,
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(bucket_id,route)
);
CREATE INDEX eve_esi_buckets_retention ON eve_esi_buckets(updated_at);
-- +goose Down
DROP TABLE eve_esi_route_usage;
DROP TABLE eve_esi_buckets;
