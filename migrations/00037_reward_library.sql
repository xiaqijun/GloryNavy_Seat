-- Module: exchange. Shared physical reward definitions; currency remains outside the library.
-- +goose Up
ALTER TABLE exchange_rewards ADD COLUMN name text NOT NULL DEFAULT '', ADD COLUMN content jsonb NOT NULL DEFAULT '{}'::jsonb, ADD COLUMN catalog_version bigint NOT NULL DEFAULT 1, ADD COLUMN archived boolean NOT NULL DEFAULT false;
UPDATE exchange_rewards SET content=jsonb_build_object('fittings','[]'::jsonb,'items',jsonb_build_array(jsonb_build_object('type_id',type_id::text,'quantity',quantity)));
ALTER TABLE exchange_rewards ADD CONSTRAINT reward_archive_listing CHECK(NOT archived OR NOT enabled);
ALTER TABLE exchange_redemptions ADD COLUMN reward_name text NOT NULL DEFAULT '', ADD COLUMN reward_content jsonb NOT NULL DEFAULT '{}'::jsonb, ADD COLUMN catalog_version bigint NOT NULL DEFAULT 1;
UPDATE exchange_redemptions SET reward_content=jsonb_build_object('fittings','[]'::jsonb,'items',jsonb_build_array(jsonb_build_object('type_id',type_id::text,'quantity',quantity)));
-- +goose Down
ALTER TABLE exchange_redemptions DROP COLUMN catalog_version, DROP COLUMN reward_content, DROP COLUMN reward_name;
ALTER TABLE exchange_rewards DROP CONSTRAINT reward_archive_listing, DROP COLUMN archived, DROP COLUMN catalog_version, DROP COLUMN content, DROP COLUMN name;
