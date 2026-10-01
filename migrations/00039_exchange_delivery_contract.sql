-- +goose Up
CREATE TABLE eve_delivery_claims (
 contract_id bigint PRIMARY KEY CHECK (contract_id > 0),
 module text NOT NULL CHECK (module IN ('welfare','exchange')),
 reference_id bigint NOT NULL CHECK (reference_id > 0),
 created_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO eve_delivery_claims(contract_id,module,reference_id)
 SELECT substring(claim_key from 10)::bigint,'welfare',case_id
 FROM welfare_claims WHERE claim_key ~ '^delivery:[0-9]+$';
CREATE INDEX eve_contract_redemption_reference ON eve_contracts(owner_id,(btrim(coalesce(payload->>'title','')))) WHERE owner_kind='character' AND in_scope;
CREATE TABLE exchange_deliveries (
 order_id bigint PRIMARY KEY REFERENCES exchange_redemptions(id),
 status text NOT NULL DEFAULT 'waiting_contract',
 evidence jsonb NOT NULL DEFAULT '[]',
 checked_at timestamptz,
 check_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO exchange_deliveries(order_id) SELECT id FROM exchange_redemptions
 WHERE state IN ('pending','cancel_requested');
CREATE INDEX exchange_delivery_due ON exchange_deliveries(check_at,order_id);
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM eve_delivery_claims WHERE module='exchange') THEN
  RAISE EXCEPTION 'Exchange delivery evidence exists; use a compatible recovery version';
 END IF;
END $$;
-- +goose StatementEnd
DROP INDEX eve_contract_redemption_reference;
DROP TABLE exchange_deliveries;
DROP TABLE eve_delivery_claims;
