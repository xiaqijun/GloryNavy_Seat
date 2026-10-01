-- Module: attendance. Immutable fleet observations and scoped battle evidence.
-- +goose Up
ALTER TABLE attendance_events ADD COLUMN ends_at timestamptz;
CREATE TABLE attendance_ships (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 event_id bigint NOT NULL,
 character_id bigint NOT NULL,
 request_key uuid NOT NULL,
 ship_type_id bigint NOT NULL CHECK(ship_type_id>0),
 solar_system_id bigint NOT NULL,
 joined_at timestamptz,
 observed_at timestamptz NOT NULL,
 fitting_state text NOT NULL DEFAULT 'pending',
 fitting jsonb NOT NULL DEFAULT '{}',
 FOREIGN KEY(event_id,character_id) REFERENCES attendance_entries(event_id,character_id),
 UNIQUE(event_id,character_id,request_key)
);
CREATE INDEX attendance_ship_history ON attendance_ships(event_id,character_id,id DESC);
CREATE TABLE attendance_battle_tasks (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 event_id bigint NOT NULL,
 character_id bigint NOT NULL,
 account_id uuid NOT NULL,
 owner_hash bytea NOT NULL,
 kind text NOT NULL CHECK(kind IN ('fitting','losses')),
 snapshot_id bigint NOT NULL DEFAULT 0,
 state text NOT NULL DEFAULT 'pending',
 reason text NOT NULL DEFAULT '',
 next_due_at timestamptz NOT NULL DEFAULT now(),
 lease_until timestamptz,
 fence bigint NOT NULL DEFAULT 0,
 failures integer NOT NULL DEFAULT 0,
 progress jsonb NOT NULL DEFAULT '{}',
 checked_at timestamptz,
 FOREIGN KEY(event_id,character_id) REFERENCES attendance_entries(event_id,character_id),
 UNIQUE(event_id,character_id,kind,snapshot_id)
);
CREATE INDEX attendance_battle_due ON attendance_battle_tasks(next_due_at) WHERE state IN ('pending','running');
CREATE TABLE attendance_losses (
 event_id bigint NOT NULL,
 character_id bigint NOT NULL,
 killmail_id bigint NOT NULL CHECK(killmail_id>0),
 occurred_at timestamptz NOT NULL,
 ship_type_id bigint NOT NULL,
 solar_system_id bigint NOT NULL,
 items jsonb NOT NULL,
 state text NOT NULL DEFAULT 'candidate' CHECK(state IN ('candidate','confirmed','rejected')),
 version bigint NOT NULL DEFAULT 1,
 FOREIGN KEY(event_id,character_id) REFERENCES attendance_entries(event_id,character_id),
 PRIMARY KEY(event_id,killmail_id)
);
CREATE UNIQUE INDEX attendance_confirmed_loss ON attendance_losses(killmail_id) WHERE state='confirmed';
-- +goose Down
DROP TABLE attendance_losses;
DROP TABLE attendance_battle_tasks;
DROP TABLE attendance_ships;
ALTER TABLE attendance_events DROP COLUMN ends_at;
