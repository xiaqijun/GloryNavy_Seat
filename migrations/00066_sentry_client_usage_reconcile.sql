-- Module: sentry. Cursor for authenticated client online-time billing.
-- +goose Up
CREATE TABLE sentry_client_usage_reconcile_state (
    id smallint PRIMARY KEY CHECK (id = 1),
    cursor text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE sentry_client_usage_reconcile_state;
