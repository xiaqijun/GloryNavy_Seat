-- Module: eve structure snapshots.
-- +goose Up
CREATE TABLE eve_structure_snapshots (
  character_id bigint PRIMARY KEY REFERENCES eve_credentials(character_id) ON DELETE CASCADE,
  generation bigint NOT NULL,
  owner_hash bytea NOT NULL,
  corporation_id bigint NOT NULL,
  corporation_name text NOT NULL,
  observed_at timestamptz NOT NULL,
  valid_until timestamptz NOT NULL,
  payload jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'array'),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX eve_structure_snapshots_corporation_idx
  ON eve_structure_snapshots(corporation_id, valid_until);

INSERT INTO eve_sync_targets(character_id, resource, generation, display_name, next_due_at, state, reason)
SELECT c.character_id, 'corporation_structures', c.grant_generation, '', now(), 'idle', ''
FROM eve_credentials c
WHERE c.state IN ('ready', 'retry')
ON CONFLICT (character_id, resource) DO NOTHING;

-- +goose Down
DROP TABLE eve_structure_snapshots;
