-- +goose Up
ALTER TABLE exchange_redemptions ADD COLUMN units integer NOT NULL DEFAULT 1 CHECK (units BETWEEN 1 AND 1000000);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM exchange_redemptions WHERE units <> 1) THEN
  RAISE EXCEPTION 'multi-unit redemptions require quantity-aware fulfillment and refunds';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE exchange_redemptions DROP COLUMN units;
