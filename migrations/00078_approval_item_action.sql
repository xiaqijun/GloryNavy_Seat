-- Module: approval. Preserve the source's latest audit action in the list
-- projection so history rows keep the same user-facing label as legacy reads.
-- +goose Up
ALTER TABLE approval_items ADD COLUMN action text NOT NULL DEFAULT '';
-- +goose Down
ALTER TABLE approval_items DROP COLUMN action;
