-- +goose Up
CREATE TABLE community_onebot_settings (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 enabled boolean NOT NULL DEFAULT false,
 api_base text NOT NULL,
 bot_qq text NOT NULL CHECK(bot_qq ~ '^[1-9][0-9]{4,11}$'),
 token_ciphertext bytea NOT NULL,
 secret_ciphertext bytea NOT NULL,
 revision bigint NOT NULL DEFAULT 1,
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE community_onebot_groups (
 group_id text PRIMARY KEY CHECK(group_id ~ '^[1-9][0-9]{4,11}$'),
 name text NOT NULL CHECK(length(name)<=128)
);
CREATE TABLE community_onebot_bindings (
 user_id uuid PRIMARY KEY REFERENCES community_profiles(user_id) ON DELETE CASCADE,
 qq_number text UNIQUE NOT NULL CHECK(qq_number ~ '^[1-9][0-9]{4,11}$'),
 qq_version bigint NOT NULL,
 bound_at timestamptz NOT NULL DEFAULT now()
);
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM community_onebot_settings) OR EXISTS(SELECT 1 FROM community_onebot_bindings) THEN
  RAISE EXCEPTION 'OneBot configuration or bindings exist; retain migration 91 and restore from a reviewed backup instead';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE community_onebot_bindings;
DROP TABLE community_onebot_groups;
DROP TABLE community_onebot_settings;
