-- Module: attendance. Fleet notice template and live PAP rollback guard.
-- +goose Up
CREATE TABLE attendance_fleet_notices (
 event_id bigint PRIMARY KEY REFERENCES attendance_events(id),
 solar_system text NOT NULL CHECK (char_length(solar_system) BETWEEN 1 AND 120),
 doctrine text NOT NULL CHECK (char_length(doctrine) BETWEEN 1 AND 500),
 meeting_point text NOT NULL DEFAULT '' CHECK (char_length(meeting_point) <= 300),
 commander text NOT NULL DEFAULT '' CHECK (char_length(commander) <= 120),
 details text NOT NULL DEFAULT '' CHECK (char_length(details) <= 3000),
 revision bigint NOT NULL DEFAULT 1,
 updated_by uuid NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE attendance_fleet_motd_updates (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 event_id bigint NOT NULL REFERENCES attendance_events(id),
 request_key uuid NOT NULL,
 actor_id uuid NOT NULL,
 source_id bigint NOT NULL,
 fleet_id bigint NOT NULL,
 generation bigint NOT NULL,
 owner_hash bytea NOT NULL,
 pap_points integer NOT NULL CHECK (pap_points BETWEEN 0 AND 10000),
 notice_revision bigint NOT NULL DEFAULT 0 CHECK (notice_revision >= 0),
 event_version bigint NOT NULL DEFAULT 0 CHECK (event_version >= 0),
 state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','preparing','sending','sent','failed','unknown','obsolete')),
 reason text NOT NULL DEFAULT '',
 updated_at timestamptz NOT NULL DEFAULT now(),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(event_id,request_key)
);
CREATE INDEX attendance_fleet_motd_state ON attendance_fleet_motd_updates(state,id);
CREATE TABLE attendance_fleet_dispatches (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 event_id bigint NOT NULL REFERENCES attendance_events(id),
 actor_id uuid NOT NULL,
 request_key uuid NOT NULL,
 corporation_id bigint NOT NULL,
 notice_revision bigint NOT NULL,
 subject text NOT NULL,
 body text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(event_id,request_key)
);
CREATE TABLE attendance_fleet_deliveries (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 dispatch_id bigint NOT NULL REFERENCES attendance_fleet_dispatches(id),
 channel text NOT NULL CHECK (channel IN ('qq','eve_mail','email')),
 target_version text NOT NULL DEFAULT '',
 account_id uuid,
 original_account_id uuid,
 state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','preparing','sending','sent','failed','unknown','skipped')),
 reason text NOT NULL DEFAULT '',
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK ((channel='email') OR account_id IS NULL)
);
CREATE INDEX attendance_fleet_delivery_state ON attendance_fleet_deliveries(state,id);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM attendance_fleet_notices)
    OR EXISTS (SELECT 1 FROM attendance_fleet_motd_updates)
    OR EXISTS (SELECT 1 FROM attendance_fleet_dispatches)
    OR EXISTS (SELECT 1 FROM attendance_audit WHERE action='capture_pap') THEN
   RAISE EXCEPTION 'Fleet notices or live PAP exist; preserve activity history';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE attendance_fleet_deliveries;
DROP TABLE attendance_fleet_dispatches;
DROP TABLE attendance_fleet_motd_updates;
DROP TABLE attendance_fleet_notices;
