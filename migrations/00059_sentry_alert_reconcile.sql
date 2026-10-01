-- +goose Up
CREATE TABLE IF NOT EXISTS sentry_alert_reconcile_state (
    id smallint PRIMARY KEY CHECK (id = 1),
    cursor text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS sentry_alert_reconcile_state;
