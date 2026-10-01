-- Module: eve. Public name cache for authorized contract views.
-- +goose Up
CREATE TABLE eve_entity_names (
 entity_id bigint PRIMARY KEY, name text NOT NULL, category text NOT NULL,
 expires_at timestamptz NOT NULL
);
CREATE INDEX eve_contract_list_filter ON eve_contracts(owner_kind,owner_id,status,contract_id DESC);
-- +goose Down
DROP INDEX eve_contract_list_filter;
DROP TABLE eve_entity_names;
