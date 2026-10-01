-- Module: eve. Metadata only; never store token values, fingerprints or raw errors.
-- +goose Up
CREATE TABLE eve_token_observations (
 character_id bigint PRIMARY KEY REFERENCES eve_credentials(character_id) ON DELETE CASCADE,
 generation bigint NOT NULL,
 observed_since timestamptz,
 access_expires_at timestamptz,
 last_used_at timestamptz,
 reuse_count bigint NOT NULL DEFAULT 0,
 last_refresh_attempt_at timestamptz,
 last_refresh_success_at timestamptz,
 last_refresh_reason text NOT NULL DEFAULT '',
 refresh_successes bigint NOT NULL DEFAULT 0,
 refresh_failures bigint NOT NULL DEFAULT 0,
 consecutive_failures bigint NOT NULL DEFAULT 0,
 last_request_at timestamptz,
 last_request_status integer NOT NULL DEFAULT 0,
 last_request_reason text NOT NULL DEFAULT '',
 network_requests bigint NOT NULL DEFAULT 0,
 cache_hits bigint NOT NULL DEFAULT 0,
 rate_limit_waits bigint NOT NULL DEFAULT 0,
 request_failures bigint NOT NULL DEFAULT 0
);
INSERT INTO eve_token_observations(character_id,generation)
SELECT character_id,grant_generation FROM eve_credentials;
CREATE TABLE eve_token_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 character_id bigint NOT NULL REFERENCES eve_credentials(character_id) ON DELETE CASCADE,
 generation bigint NOT NULL,
 occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 outcome text NOT NULL CHECK(outcome IN ('authorized','refresh_success','refresh_failed')),
 reason text NOT NULL DEFAULT '',
 duration_ms bigint NOT NULL DEFAULT 0 CHECK(duration_ms >= 0)
);
CREATE INDEX eve_token_events_character ON eve_token_events(character_id,id DESC);
CREATE INDEX eve_token_events_retention ON eve_token_events(occurred_at);
-- +goose Down
DROP TABLE eve_token_events;
DROP TABLE eve_token_observations;
