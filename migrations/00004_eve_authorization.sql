-- Module: eve. Encrypted credentials and a replaceable authorization snapshot.
-- +goose Up
CREATE TABLE eve_credentials (
    character_id bigint PRIMARY KEY CHECK (character_id > 0),
    owner_hash bytea NOT NULL CHECK (octet_length(owner_hash) = 32),
    sealed bytea NOT NULL,
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'ready', 'retry', 'reauthorize')),
    next_sync_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX eve_credentials_due ON eve_credentials(next_sync_at) WHERE state <> 'reauthorize';
CREATE TABLE eve_role_snapshots (
    character_id bigint PRIMARY KEY REFERENCES eve_credentials(character_id) ON DELETE CASCADE,
    owner_hash bytea NOT NULL,
    corporation_id bigint NOT NULL CHECK (corporation_id > 0),
    corporation_name text NOT NULL,
    alliance_id bigint NOT NULL DEFAULT 0,
    ceo_id bigint NOT NULL CHECK (ceo_id > 0),
    roles text[] NOT NULL,
    roles_at_hq text[] NOT NULL,
    roles_at_base text[] NOT NULL,
    roles_at_other text[] NOT NULL,
    synced_at timestamptz NOT NULL,
    valid_until timestamptz NOT NULL
);
-- +goose Down
DROP TABLE eve_role_snapshots;
DROP TABLE eve_credentials;
