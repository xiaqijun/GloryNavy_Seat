-- +goose Up
ALTER TABLE exchange_redemptions ADD COLUMN settlement_reference text;
UPDATE exchange_redemptions SET settlement_reference='GNV-EX-' || id::text;
ALTER TABLE exchange_redemptions
 ALTER COLUMN settlement_reference SET NOT NULL,
 ALTER COLUMN settlement_reference SET DEFAULT ('GNV-EX-' || upper(gen_random_uuid()::text)),
 ADD CONSTRAINT exchange_settlement_reference_unique UNIQUE(settlement_reference),
 ADD CONSTRAINT exchange_settlement_reference_format CHECK (
  settlement_reference ~ '^GNV-EX-([1-9][0-9]*|[0-9A-F]{8}-[0-9A-F]{4}-4[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12})$'
 );

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM exchange_redemptions WHERE settlement_reference <> 'GNV-EX-' || id::text) THEN
  RAISE EXCEPTION 'Random settlement references exist; use a compatible recovery version';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE exchange_redemptions DROP COLUMN settlement_reference;
