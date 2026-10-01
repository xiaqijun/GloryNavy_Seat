-- Module: eve. Requested scope snapshots and granted scope metadata.
-- +goose Up
ALTER TABLE eve_login_flows ADD COLUMN requested_scopes text[] NOT NULL DEFAULT '{}';
ALTER TABLE eve_credentials ADD COLUMN scopes text[] NOT NULL DEFAULT '{}';
-- +goose Down
ALTER TABLE eve_credentials DROP COLUMN scopes;
ALTER TABLE eve_login_flows DROP COLUMN requested_scopes;
