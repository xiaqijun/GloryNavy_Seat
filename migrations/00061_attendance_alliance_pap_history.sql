-- Keep a durable completeness/version marker for every successfully published
-- alliance PAP month so administrators can settle an older snapshot safely.
-- +goose Up
CREATE TABLE attendance_alliance_pap_sync_history (
  month date PRIMARY KEY CHECK (month = date_trunc('month', month)::date),
  state text NOT NULL CHECK (state IN ('ready','error')),
  complete boolean NOT NULL DEFAULT false,
  records_total integer NOT NULL DEFAULT 0 CHECK (records_total >= 0),
  last_synced_at timestamptz,
  last_error text,
  version bigint NOT NULL DEFAULT 1 CHECK (version > 0)
);

INSERT INTO attendance_alliance_pap_sync_history
  (month, state, complete, records_total, last_synced_at, version)
SELECT month, 'ready', true, count(*)::integer, max(synced_at), 1
FROM attendance_alliance_pap_snapshot
GROUP BY month
ON CONFLICT (month) DO NOTHING;

CREATE INDEX attendance_alliance_pap_sync_history_ready_idx
  ON attendance_alliance_pap_sync_history (month DESC)
  WHERE state = 'ready' AND complete;

-- +goose Down
DROP TABLE attendance_alliance_pap_sync_history;
