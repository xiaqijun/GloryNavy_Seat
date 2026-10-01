-- Module: eve. Only short-lived authorization attempts; no EVE tokens are persisted.
-- +goose Up
CREATE TABLE eve_login_flows (
    state_hash bytea PRIMARY KEY CHECK (octet_length(state_hash) = 32),
    browser_hash bytea NOT NULL CHECK (octet_length(browser_hash) = 32),
    session_hash bytea NOT NULL CHECK (octet_length(session_hash) = 32),
    verifier text NOT NULL,
    expires_at timestamptz NOT NULL DEFAULT (now() + interval '10 minutes')
);
CREATE INDEX eve_login_flows_expiry ON eve_login_flows(expires_at);
-- +goose Down
DROP TABLE eve_login_flows;
