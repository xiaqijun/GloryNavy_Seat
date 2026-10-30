-- Module: attendance. Preserve structured in-game links alongside legacy text.
-- +goose Up
ALTER TABLE attendance_fleet_notices ADD COLUMN links jsonb NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
 IF EXISTS (SELECT 1 FROM attendance_fleet_notices WHERE links <> '{}'::jsonb) THEN
  RAISE EXCEPTION 'fleet notice links exist; rollback would lose data';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE attendance_fleet_notices DROP COLUMN links;
