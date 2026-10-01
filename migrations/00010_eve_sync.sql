-- Module: eve. River owns its own migrations; these are business sync records.
-- +goose Up
ALTER TABLE eve_credentials ADD COLUMN grant_generation bigint NOT NULL DEFAULT 1;
ALTER TABLE eve_credentials ADD COLUMN refresh_revision bigint NOT NULL DEFAULT 0;
ALTER TABLE eve_credentials ADD COLUMN roles_not_before timestamptz;
CREATE TABLE eve_sync_targets (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 character_id bigint NOT NULL REFERENCES eve_credentials(character_id) ON DELETE CASCADE,
 resource text NOT NULL,
 generation bigint NOT NULL,
 display_name text NOT NULL DEFAULT '',
 state text NOT NULL DEFAULT 'idle' CHECK(state IN ('idle','queued','running','deferred','failed','blocked')),
 reason text NOT NULL DEFAULT '',
 next_due_at timestamptz NOT NULL DEFAULT now(),
 last_attempt_at timestamptz,
 last_success_at timestamptz,
 valid_until timestamptz,
 content_updated_at timestamptz,
 active_job_id bigint,
 completed_job_id bigint,
 fence bigint NOT NULL DEFAULT 0,
 lease_until timestamptz,
 failures integer NOT NULL DEFAULT 0,
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(character_id,resource)
);
CREATE INDEX eve_sync_due ON eve_sync_targets(next_due_at,id) WHERE state <> 'blocked';
CREATE TABLE eve_sync_runs (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 target_id bigint NOT NULL REFERENCES eve_sync_targets(id) ON DELETE CASCADE,
 job_id bigint NOT NULL,
 fence bigint NOT NULL,
 started_at timestamptz NOT NULL DEFAULT now(),
 finished_at timestamptz,
 outcome text NOT NULL DEFAULT 'running',
 reason text NOT NULL DEFAULT '',
 http_status integer NOT NULL DEFAULT 0,
 UNIQUE(target_id,fence)
);
CREATE INDEX eve_sync_run_history ON eve_sync_runs(target_id,id DESC);
CREATE TABLE eve_sync_audit (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 user_id uuid NOT NULL,
 character_id bigint NOT NULL,
 target_id bigint NOT NULL,
 action text NOT NULL,
 outcome text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE eve_character_profiles (
 character_id bigint PRIMARY KEY REFERENCES eve_credentials(character_id) ON DELETE CASCADE,
 name text NOT NULL,
 corporation_id bigint NOT NULL,
 alliance_id bigint NOT NULL DEFAULT 0,
 checked_at timestamptz NOT NULL,
 valid_until timestamptz NOT NULL
);
CREATE TABLE eve_esi_cache (
 cache_key text PRIMARY KEY,
 character_id bigint REFERENCES eve_credentials(character_id) ON DELETE CASCADE,
 generation bigint NOT NULL DEFAULT 0,
 body bytea NOT NULL CHECK(octet_length(body)<=2097152),
 etag text NOT NULL DEFAULT '',
 last_modified text NOT NULL DEFAULT '',
 content_updated_at timestamptz NOT NULL DEFAULT now(),
 expires_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX eve_esi_cache_expiry ON eve_esi_cache(expires_at);
CREATE TABLE eve_esi_routes (route_key text PRIMARY KEY, group_name text NOT NULL);
CREATE TABLE eve_esi_limits (
 limit_key text PRIMARY KEY,
 blocked_until timestamptz NOT NULL DEFAULT 'epoch',
 next_request_at timestamptz NOT NULL DEFAULT 'epoch',
 remaining bigint NOT NULL DEFAULT -1,
 capacity bigint NOT NULL DEFAULT 0,
 window_seconds bigint NOT NULL DEFAULT 60,
 reset_at timestamptz NOT NULL DEFAULT 'epoch',
 updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO eve_sync_targets(character_id,resource,generation,next_due_at,state,reason)
SELECT character_id,resource,grant_generation,next_sync_at,
 CASE WHEN state='reauthorize' AND resource='authorization' THEN 'blocked' ELSE 'idle' END,
 CASE WHEN state='reauthorize' AND resource='authorization' THEN 'reauthorize' ELSE '' END
FROM eve_credentials CROSS JOIN (VALUES ('profile'),('authorization')) resources(resource);
-- +goose Down
DROP TABLE eve_esi_routes, eve_esi_limits, eve_esi_cache, eve_character_profiles, eve_sync_audit, eve_sync_runs, eve_sync_targets;
ALTER TABLE eve_credentials DROP COLUMN roles_not_before, DROP COLUMN refresh_revision, DROP COLUMN grant_generation;
