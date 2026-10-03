-- Module: sentry. Persist the administrator-controlled charging switch.
-- The switch defaults to off so deploying this migration cannot start billing.
-- +goose Up
ALTER TABLE sentry_alert_pricing
    ADD COLUMN charging_enabled boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE sentry_alert_pricing
    DROP COLUMN charging_enabled;
