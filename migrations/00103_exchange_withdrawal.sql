-- +goose Up
ALTER TABLE exchange_deliveries ADD COLUMN started_at timestamptz,
 ADD COLUMN started_by uuid REFERENCES identity_users(id);

-- Existing contract/batch evidence must never become an unstarted order.
UPDATE exchange_deliveries d SET started_at=coalesce(d.checked_at,r.created_at)
FROM exchange_redemptions r WHERE r.id=d.order_id AND
 (d.evidence <> '[]'::jsonb OR EXISTS (
 SELECT 1 FROM welfare_contract_settlement_items i WHERE i.source='exchange' AND i.source_id=r.id));

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM exchange_deliveries WHERE started_at IS NOT NULL)
 OR EXISTS (SELECT 1 FROM exchange_shop_audit WHERE payload->>'action'='withdrawn') THEN
  RAISE EXCEPTION 'delivery starts or owner withdrawals require the withdrawal-aware application';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE exchange_deliveries DROP COLUMN started_at, DROP COLUMN started_by;
