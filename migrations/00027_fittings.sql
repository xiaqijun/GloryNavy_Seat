-- Modules: eve (authorized snapshots), fittings (site-owned drafts).
-- +goose Up
CREATE TABLE eve_fitting_snapshots(character_id bigint NOT NULL REFERENCES eve_credentials(character_id) ON DELETE CASCADE,resource text NOT NULL CHECK(resource IN ('fittings','skills')),generation bigint NOT NULL,observed_at timestamptz NOT NULL,payload jsonb NOT NULL,PRIMARY KEY(character_id,resource));
CREATE TABLE fittings_drafts(id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,account_id uuid NOT NULL,name text NOT NULL CHECK(char_length(name) BETWEEN 1 AND 80),fit jsonb NOT NULL,version bigint NOT NULL DEFAULT 1,request_key uuid NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),UNIQUE(account_id,request_key));
CREATE INDEX fittings_drafts_account ON fittings_drafts(account_id,id DESC);
-- +goose Down
DELETE FROM eve_sync_targets WHERE resource IN ('fittings','skills');
DROP TABLE fittings_drafts;
DROP TABLE eve_fitting_snapshots;
