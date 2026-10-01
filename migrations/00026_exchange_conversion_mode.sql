-- Module: exchange. Existing coin awards remain intact; no historical backfill.
-- +goose Up
ALTER TABLE exchange_source_rates ADD COLUMN conversion_mode text NOT NULL DEFAULT 'manual' CHECK(conversion_mode IN ('manual','automatic'));
-- +goose Down
ALTER TABLE exchange_source_rates DROP COLUMN conversion_mode;
