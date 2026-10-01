-- +goose Up
ALTER TABLE exchange_redemptions DROP CONSTRAINT exchange_redemptions_state_check;
ALTER TABLE exchange_redemptions ADD CONSTRAINT exchange_redemptions_state_check
  CHECK (state IN ('pending','cancel_requested','fulfilled','cancelled'));

-- +goose Down
-- Do not turn cancellation requests into refundable or deliverable orders silently.
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM exchange_redemptions WHERE state='cancel_requested') THEN
    RAISE EXCEPTION 'Resolve pending cancellation requests before rolling back';
  END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE exchange_redemptions DROP CONSTRAINT exchange_redemptions_state_check;
ALTER TABLE exchange_redemptions ADD CONSTRAINT exchange_redemptions_state_check
  CHECK (state IN ('pending','fulfilled','cancelled'));
