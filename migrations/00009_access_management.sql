-- Module: access. Optimistic concurrency for role administration.
-- +goose Up
ALTER TABLE access_roles ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK (version > 0);
-- +goose Down
ALTER TABLE access_roles DROP COLUMN version;
