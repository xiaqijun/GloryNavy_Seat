-- Module: welfare. Existing approvals are not linked or completed automatically.
-- +goose Up
ALTER TABLE welfare_cases ADD COLUMN delivery_check_at timestamptz;
CREATE INDEX welfare_delivery_due ON welfare_cases(delivery_check_at,id)
 WHERE state='executing' AND detail ? 'delivery';
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM welfare_cases WHERE detail ? 'delivery') THEN
  RAISE EXCEPTION 'Contract delivery evidence exists; use a compatible recovery version';
 END IF;
END $$;
-- +goose StatementEnd
DROP INDEX welfare_delivery_due;
ALTER TABLE welfare_cases DROP COLUMN delivery_check_at;
