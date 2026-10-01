-- Modules: attendance (events/audit), eve (online observations).
-- +goose Up
CREATE TABLE attendance_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 corporation_id bigint NOT NULL CHECK(corporation_id>0),
 title text NOT NULL CHECK(char_length(title) BETWEEN 1 AND 80),
 starts_at timestamptz NOT NULL,
 created_by uuid NOT NULL,
 request_key uuid NOT NULL,
 state text NOT NULL DEFAULT 'open' CHECK(state IN ('open','closed')),
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(created_by,request_key)
);
CREATE INDEX attendance_corporation_events ON attendance_events(corporation_id,id DESC);
CREATE TABLE attendance_entries (
 event_id bigint NOT NULL REFERENCES attendance_events(id),
 character_id bigint NOT NULL CHECK(character_id>0),
 account_id uuid,
 character_name text NOT NULL,
 source text NOT NULL CHECK(source IN ('fleet','manual')),
 present boolean NOT NULL DEFAULT true,
 recorded_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,character_id)
);
CREATE INDEX attendance_account_events ON attendance_entries(account_id,event_id DESC) WHERE present;
CREATE TABLE attendance_audit (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 event_id bigint NOT NULL REFERENCES attendance_events(id),
 actor_id uuid NOT NULL,
 action text NOT NULL,
 request_key uuid NOT NULL,
 payload jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(event_id,request_key)
);
CREATE TABLE eve_online_samples (
 character_id bigint NOT NULL REFERENCES eve_credentials(character_id) ON DELETE CASCADE,
 generation bigint NOT NULL,
 corporation_id bigint NOT NULL DEFAULT 0,
 observed_at timestamptz NOT NULL,
 online boolean,
 PRIMARY KEY(character_id,observed_at)
);
CREATE INDEX eve_online_retention ON eve_online_samples(observed_at);
-- Online targets are seeded by the enabled attendance runtime, not by migration.
-- +goose Down
DELETE FROM eve_sync_targets WHERE resource='online';
DROP TABLE eve_online_samples;
DROP TABLE attendance_audit;
DROP TABLE attendance_entries;
DROP TABLE attendance_events;
