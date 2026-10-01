-- Module: sentry. Platform-managed client keys for the EVE warning service.
-- Plaintext secrets are never persisted; only a SHA-256 digest and prefix are kept.
-- +goose Up
CREATE TABLE sentry_keys (
    id uuid PRIMARY KEY,
    account_id uuid NOT NULL REFERENCES identity_users(id),
    operation_id uuid NOT NULL UNIQUE,
    remote_key_id text NOT NULL DEFAULT '' CHECK (char_length(remote_key_id) <= 128 AND remote_key_id !~ '[\r\n]'),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80 AND name !~ '[\r\n]'),
    key_prefix text NOT NULL CHECK (char_length(key_prefix) BETWEEN 4 AND 32 AND key_prefix !~ '[\r\n]'),
    key_hash bytea NOT NULL CHECK (octet_length(key_hash) = 32),
    permissions text[] NOT NULL CHECK (cardinality(permissions) BETWEEN 1 AND 2),
    status text NOT NULL CHECK (status IN ('creating','active','revoking','revoked','sync_error')),
    remote_version bigint NOT NULL DEFAULT 0 CHECK (remote_version >= 0),
    last_error text NOT NULL DEFAULT '' CHECK (char_length(last_error) <= 500 AND last_error !~ '[\r\n]'),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz
);
CREATE INDEX sentry_keys_account_created ON sentry_keys(account_id, created_at DESC);
CREATE UNIQUE INDEX sentry_keys_account_active_name ON sentry_keys(account_id, lower(name)) WHERE status <> 'revoked';

CREATE TABLE sentry_key_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    key_id uuid NOT NULL REFERENCES sentry_keys(id) ON DELETE CASCADE,
    account_id uuid NOT NULL REFERENCES identity_users(id),
    action text NOT NULL CHECK (action IN ('creating','active','revoke_requested','revoked','sync_error')),
    detail jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sentry_key_events_account_created ON sentry_key_events(account_id, created_at DESC);

-- +goose Down
DROP TABLE sentry_key_events;
DROP TABLE sentry_keys;
