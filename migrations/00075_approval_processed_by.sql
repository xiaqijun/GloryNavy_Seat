-- Module: approval. Preserve the actor used by the legacy history/mine filter.
-- +goose Up
ALTER TABLE approval_items ADD COLUMN processed_by jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(processed_by) = 'array');
CREATE INDEX approval_items_processed_by ON approval_items USING GIN (processed_by);

-- +goose Down
DROP INDEX approval_items_processed_by;
ALTER TABLE approval_items DROP COLUMN processed_by;
