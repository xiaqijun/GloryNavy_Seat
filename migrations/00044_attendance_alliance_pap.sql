-- attendance-owned current-month alliance PAP snapshots.
-- Local roll-call PAP stays in attendance_pap_awards and is not mixed here.
-- +goose Up
CREATE TABLE attendance_alliance_pap_snapshot (
  month date NOT NULL,
  character_id bigint NOT NULL,
  character_name text NOT NULL,
  pap numeric(18,2) NOT NULL CHECK (pap >= 0),
  account_id uuid,
  synced_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (month, character_id)
);
CREATE INDEX attendance_alliance_pap_account_month_idx
  ON attendance_alliance_pap_snapshot (account_id, month);

CREATE TABLE attendance_alliance_pap_sync (
  singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
  month date,
  state text NOT NULL CHECK (state IN ('idle','syncing','ready','error')),
  complete boolean NOT NULL DEFAULT false,
  records_total integer NOT NULL DEFAULT 0 CHECK (records_total >= 0),
  last_synced_at timestamptz,
  last_error text,
  version bigint NOT NULL DEFAULT 1 CHECK (version > 0)
);
INSERT INTO attendance_alliance_pap_sync(singleton,state) VALUES (true,'idle');

-- +goose Down
DROP TABLE attendance_alliance_pap_sync;
DROP TABLE attendance_alliance_pap_snapshot;
