-- Module: exchange. Preserve the pricing policy used by each prepaid alert grant.
-- +goose Up
ALTER TABLE exchange_alert_grants
    ADD COLUMN price_version text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE exchange_alert_grants
    DROP COLUMN price_version;
