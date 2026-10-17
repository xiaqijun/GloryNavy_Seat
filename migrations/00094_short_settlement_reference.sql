-- Shared database numbering utility; references remain in module-private tables.
-- +goose Up
CREATE SEQUENCE gn_settlement_reference_seq AS bigint NO CYCLE;
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION gn_settlement_reference(prefix text, issued_at timestamptz)
RETURNS text LANGUAGE sql VOLATILE STRICT AS $$
 WITH serial AS MATERIALIZED (SELECT nextval('gn_settlement_reference_seq')::text AS n)
 SELECT prefix || '-' || to_char(issued_at AT TIME ZONE 'UTC', 'YYYYMMDD') || '-' ||
        lpad(n, greatest(6, length(n)), '0') FROM serial
$$;
-- +goose StatementEnd

ALTER TABLE exchange_redemptions
 DROP CONSTRAINT exchange_settlement_reference_format,
 ADD CONSTRAINT exchange_settlement_reference_format CHECK (
  settlement_reference ~ '^(GNV-EX-([1-9][0-9]*|[0-9A-F]{8}-[0-9A-F]{4}-4[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12})|EX-[0-9]{8}-([0-9]{6,19}|[0-9A-F]{8}-[0-9A-F]{4}-4[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12}))$'
 );
ALTER TABLE welfare_cases
 DROP CONSTRAINT welfare_settlement_reference_format,
 ADD CONSTRAINT welfare_settlement_reference_format CHECK (
  settlement_reference ~ '^(GNV-WF-[1-9][0-9]*|(SRP|PVP|WF)-[0-9]{8}-([0-9]{6,19}|[0-9A-F]{8}-[0-9A-F]{4}-4[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12}))$'
 );
ALTER TABLE welfare_contract_settlement_batches
 DROP CONSTRAINT welfare_contract_settlement_reference_format,
 ADD CONSTRAINT welfare_contract_settlement_reference_format CHECK (
  settlement_reference IS NULL OR settlement_reference ~ '^BATCH-[0-9]{8}-([0-9]{6,19}|[0-9A-F]{8}-[0-9A-F]{4}-4[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12})$'
 );

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM exchange_redemptions WHERE settlement_reference ~ '^EX-[0-9]{8}-[0-9]{6,19}$')
 OR EXISTS(SELECT 1 FROM welfare_cases WHERE settlement_reference ~ '^(SRP|PVP|WF)-[0-9]{8}-[0-9]{6,19}$')
 OR EXISTS(SELECT 1 FROM welfare_contract_settlement_batches WHERE settlement_reference ~ '^BATCH-[0-9]{8}-[0-9]{6,19}$') THEN
  RAISE EXCEPTION 'Short settlement references exist; use a compatible recovery version';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE exchange_redemptions
 DROP CONSTRAINT exchange_settlement_reference_format,
 ADD CONSTRAINT exchange_settlement_reference_format CHECK (
  settlement_reference ~ '^(GNV-EX-([1-9][0-9]*|[0-9A-F]{8}-[0-9A-F]{4}-4[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12})|EX-[0-9]{8}-[0-9A-F]{8}-[0-9A-F]{4}-4[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12})$'
 );
ALTER TABLE welfare_cases
 DROP CONSTRAINT welfare_settlement_reference_format,
 ADD CONSTRAINT welfare_settlement_reference_format CHECK (
  settlement_reference ~ '^(GNV-WF-[1-9][0-9]*|(SRP|PVP|WF)-[0-9]{8}-[0-9A-F]{8}-[0-9A-F]{4}-4[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12})$'
 );
ALTER TABLE welfare_contract_settlement_batches
 DROP CONSTRAINT welfare_contract_settlement_reference_format,
 ADD CONSTRAINT welfare_contract_settlement_reference_format CHECK (
  settlement_reference IS NULL OR settlement_reference ~ '^BATCH-[0-9]{8}-[0-9A-F]{8}-[0-9A-F]{4}-4[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12}$'
 );
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION gn_settlement_reference(prefix text, issued_at timestamptz)
RETURNS text LANGUAGE sql VOLATILE STRICT AS $$
 SELECT prefix || '-' || to_char(issued_at AT TIME ZONE 'UTC', 'YYYYMMDD') || '-' || upper(gen_random_uuid()::text)
$$;
-- +goose StatementEnd
DROP SEQUENCE gn_settlement_reference_seq;
