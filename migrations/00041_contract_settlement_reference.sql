-- Shared settlement format; each module retains ownership of its stored reference.
-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION gn_settlement_reference(prefix text, issued_at timestamptz)
RETURNS text LANGUAGE sql VOLATILE STRICT AS $$
 SELECT prefix || '-' || to_char(issued_at AT TIME ZONE 'UTC', 'YYYYMMDD') || '-' || upper(gen_random_uuid()::text)
$$;
-- +goose StatementEnd
ALTER TABLE exchange_redemptions
 ALTER COLUMN settlement_reference SET DEFAULT gn_settlement_reference('EX', now()),
 DROP CONSTRAINT exchange_settlement_reference_format,
 ADD CONSTRAINT exchange_settlement_reference_format CHECK (
  settlement_reference ~ '^(GNV-EX-([1-9][0-9]*|[0-9A-F]{8}-[0-9A-F]{4}-4[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12})|EX-[0-9]{8}-[0-9A-F]{8}-[0-9A-F]{4}-4[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12})$'
 );

ALTER TABLE welfare_cases ADD COLUMN settlement_reference text;
UPDATE welfare_cases SET settlement_reference='GNV-WF-' || id::text;
ALTER TABLE welfare_cases
 ALTER COLUMN settlement_reference SET NOT NULL,
 ADD CONSTRAINT welfare_settlement_reference_unique UNIQUE(settlement_reference),
 ADD CONSTRAINT welfare_settlement_reference_format CHECK (
  settlement_reference ~ '^(GNV-WF-[1-9][0-9]*|(SRP|PVP|WF)-[0-9]{8}-[0-9A-F]{8}-[0-9A-F]{4}-4[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12})$'
 );
-- +goose StatementBegin
CREATE FUNCTION welfare_assign_settlement_reference() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.settlement_reference IS NULL THEN
  NEW.settlement_reference := gn_settlement_reference(
   CASE NEW.kind WHEN 'srp' THEN 'SRP' WHEN 'solo' THEN 'PVP' ELSE 'WF' END, NEW.created_at);
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER welfare_assign_settlement_reference BEFORE INSERT ON welfare_cases
 FOR EACH ROW EXECUTE FUNCTION welfare_assign_settlement_reference();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM welfare_cases WHERE settlement_reference <> 'GNV-WF-' || id::text)
 OR EXISTS(SELECT 1 FROM exchange_redemptions WHERE settlement_reference LIKE 'EX-%') THEN
  RAISE EXCEPTION 'Dated settlement references exist; use a compatible recovery version';
 END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER welfare_assign_settlement_reference ON welfare_cases;
DROP FUNCTION welfare_assign_settlement_reference();
ALTER TABLE welfare_cases DROP COLUMN settlement_reference;
ALTER TABLE exchange_redemptions
 ALTER COLUMN settlement_reference SET DEFAULT ('GNV-EX-' || upper(gen_random_uuid()::text)),
 DROP CONSTRAINT exchange_settlement_reference_format,
 ADD CONSTRAINT exchange_settlement_reference_format CHECK (
  settlement_reference ~ '^GNV-EX-([1-9][0-9]*|[0-9A-F]{8}-[0-9A-F]{4}-4[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12})$'
 );
DROP FUNCTION gn_settlement_reference(text, timestamptz);
