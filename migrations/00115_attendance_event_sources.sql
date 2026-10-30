-- Module: attendance. Each operator remembers their own source for each event.
-- +goose Up
CREATE TABLE attendance_event_sources (
 event_id bigint NOT NULL REFERENCES attendance_events(id),
 account_id uuid NOT NULL,
 source_id bigint NOT NULL CHECK (source_id > 0),
 creation_source_id bigint CHECK (creation_source_id > 0),
 revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (event_id, account_id)
);
CREATE INDEX attendance_event_sources_account ON attendance_event_sources(account_id,event_id);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM attendance_event_sources) THEN
   RAISE EXCEPTION 'Saved fleet source selections exist; preserve account preferences';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE attendance_event_sources;
