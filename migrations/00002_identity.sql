-- Module: identity. Foundation schema_version remains 1.
-- +goose Up
CREATE TABLE identity_users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE identity_characters (
    character_id bigint PRIMARY KEY CHECK (character_id > 0),
    user_id uuid NOT NULL REFERENCES identity_users(id),
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    owner_hash bytea NOT NULL CHECK (octet_length(owner_hash) = 32),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'blocked')),
    verified_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, character_id)
);
CREATE TABLE identity_sessions (
    token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    user_id uuid NOT NULL,
    character_id bigint NOT NULL,
    csrf_token text NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (user_id, character_id) REFERENCES identity_characters(user_id, character_id)
);
CREATE INDEX identity_sessions_expiry ON identity_sessions(expires_at);
CREATE INDEX identity_sessions_user ON identity_sessions(user_id);
-- +goose Down
DROP TABLE identity_sessions;
DROP TABLE identity_characters;
DROP TABLE identity_users;
