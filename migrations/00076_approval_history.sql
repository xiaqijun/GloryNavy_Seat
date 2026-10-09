-- Module: approval. Preserve the legacy distinction between a pending bucket
-- and records that are also visible in the history view after processing.
-- +goose Up
ALTER TABLE approval_items ADD COLUMN history boolean NOT NULL DEFAULT false;
CREATE INDEX approval_items_history_time ON approval_items(history, occurred_at, id);

-- +goose Down
DROP INDEX approval_items_history_time;
ALTER TABLE approval_items DROP COLUMN history;
