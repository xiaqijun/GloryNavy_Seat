-- +goose Up
ALTER TABLE community_qq_group_settings
    ADD COLUMN join_url text NOT NULL DEFAULT '' CHECK (char_length(join_url) <= 512);

-- +goose Down
ALTER TABLE community_qq_group_settings DROP COLUMN join_url;
