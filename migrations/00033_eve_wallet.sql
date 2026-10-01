-- EVE owns ISK observations. These are not exchange coin balances or welfare receipts.
-- +goose Up
CREATE TABLE eve_wallet_cursors (
 target_id bigint PRIMARY KEY REFERENCES eve_sync_targets(id) ON DELETE CASCADE,
 generation bigint NOT NULL, owner_id bigint NOT NULL, payload jsonb NOT NULL
);
CREATE TABLE eve_wallet_observations (
 owner_kind text NOT NULL CHECK(owner_kind IN ('character','corporation')),
 owner_id bigint NOT NULL CHECK(owner_id>0), division integer NOT NULL CHECK(division BETWEEN 0 AND 7),
 kind text NOT NULL CHECK(kind IN ('balance','journal','transactions','divisions')),
 record_id bigint NOT NULL CHECK(record_id>=0),
 source_character_id bigint NOT NULL REFERENCES eve_credentials(character_id) ON DELETE CASCADE,
 owner_hash bytea NOT NULL, observed_at timestamptz NOT NULL, occurred_at timestamptz,
 payload jsonb NOT NULL,
 PRIMARY KEY(owner_kind,owner_id,division,kind,record_id)
);
CREATE INDEX eve_wallet_history ON eve_wallet_observations(owner_kind,owner_id,division,kind,occurred_at DESC,record_id DESC);
-- +goose Down
DELETE FROM eve_sync_targets WHERE resource LIKE 'wallet_%' OR resource LIKE 'corporation_wallet_%';
DROP TABLE eve_wallet_cursors;
DROP TABLE eve_wallet_observations;
