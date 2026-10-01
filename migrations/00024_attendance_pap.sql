-- Module: attendance. Character PAP balances and append-only adjustments.
-- +goose Up
ALTER TABLE attendance_events ADD COLUMN pap_points integer NOT NULL DEFAULT 1 CHECK(pap_points BETWEEN 1 AND 10000);
ALTER TABLE attendance_events ADD COLUMN pap_issued boolean NOT NULL DEFAULT false;
CREATE TABLE attendance_pap_awards (
 event_id bigint NOT NULL,
 character_id bigint NOT NULL,
 account_id uuid NOT NULL,
 character_name text NOT NULL,
 points integer NOT NULL CHECK(points BETWEEN 0 AND 10000),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(event_id,character_id),
 FOREIGN KEY(event_id,character_id) REFERENCES attendance_entries(event_id,character_id)
);
CREATE INDEX attendance_pap_account ON attendance_pap_awards(account_id,event_id);
CREATE TABLE attendance_pap_ledger (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 event_id bigint NOT NULL,
 character_id bigint NOT NULL,
 account_id uuid NOT NULL,
 actor_id uuid NOT NULL,
 request_key uuid NOT NULL,
 delta integer NOT NULL CHECK(delta<>0),
 balance integer NOT NULL CHECK(balance BETWEEN 0 AND 10000),
 reason text NOT NULL CHECK(char_length(reason) BETWEEN 1 AND 200),
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(event_id,character_id) REFERENCES attendance_pap_awards(event_id,character_id),
 UNIQUE(event_id,character_id,request_key)
);
-- +goose Down
DROP TABLE attendance_pap_ledger;
DROP TABLE attendance_pap_awards;
ALTER TABLE attendance_events DROP COLUMN pap_issued;
ALTER TABLE attendance_events DROP COLUMN pap_points;
