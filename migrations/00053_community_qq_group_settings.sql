-- Module: community. Persist the QQ Bot group allow-list managed by administrators.
-- App credentials remain server-only environment configuration.
-- +goose Up
CREATE TABLE community_qq_group_settings (
 group_openid text PRIMARY KEY CHECK(length(group_openid)>0 AND length(group_openid)<=256 AND group_openid !~ '^[0-9]+$' AND group_openid !~ '[\r\n]'),
 label text NOT NULL DEFAULT '' CHECK(length(label)<=128),
 enabled boolean NOT NULL DEFAULT true,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX community_qq_group_settings_enabled ON community_qq_group_settings(enabled, group_openid);
CREATE TABLE community_qq_group_settings_meta (
 id boolean PRIMARY KEY DEFAULT true CHECK(id=true),
 updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE community_qq_group_settings_meta;
DROP TABLE community_qq_group_settings;
