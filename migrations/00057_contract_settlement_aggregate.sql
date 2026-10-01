-- Module: welfare. Persist the frozen one-contract settlement projection.
-- +goose Up
ALTER TABLE welfare_contract_settlement_batches
    ADD COLUMN settlement_reference text,
    ADD COLUMN account_id uuid REFERENCES identity_users(id),
    ADD COLUMN isk_minor bigint NOT NULL DEFAULT 0 CHECK (isk_minor >= 0),
    ADD COLUMN items jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN recipient_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN contract_id bigint,
    ADD COLUMN contract_recipient_id bigint,
    ADD CONSTRAINT welfare_contract_settlement_reference_unique UNIQUE (settlement_reference),
    ADD CONSTRAINT welfare_contract_settlement_reference_format CHECK (
        settlement_reference IS NULL OR settlement_reference ~ '^BATCH-[0-9]{8}-[0-9A-F]{8}-[0-9A-F]{4}-4[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12}$'
    );

-- +goose Down
ALTER TABLE welfare_contract_settlement_batches
    DROP CONSTRAINT welfare_contract_settlement_reference_format,
    DROP CONSTRAINT welfare_contract_settlement_reference_unique,
    DROP COLUMN contract_recipient_id,
    DROP COLUMN contract_id,
    DROP COLUMN recipient_ids,
    DROP COLUMN items,
    DROP COLUMN isk_minor,
    DROP COLUMN account_id,
    DROP COLUMN settlement_reference;
