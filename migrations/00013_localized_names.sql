-- Module: eve. Keep localized item names separate from English entity names.
-- +goose Up
ALTER TABLE eve_entity_names ADD COLUMN language text NOT NULL DEFAULT 'en';
ALTER TABLE eve_entity_names DROP CONSTRAINT eve_entity_names_pkey;
ALTER TABLE eve_entity_names ADD PRIMARY KEY(entity_id,language);
-- +goose Down
DELETE FROM eve_entity_names WHERE language<>'en';
ALTER TABLE eve_entity_names DROP CONSTRAINT eve_entity_names_pkey;
ALTER TABLE eve_entity_names DROP COLUMN language;
ALTER TABLE eve_entity_names ADD PRIMARY KEY(entity_id);
