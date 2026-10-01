-- Module: community. Persist official QQ Bot settings entered by administrators.
-- The secret is stored as authenticated ciphertext; plaintext never reaches
-- the database or a browser response.
-- +goose Up
CREATE TABLE community_qq_bot_settings (
 id boolean PRIMARY KEY DEFAULT true CHECK(id=true),
 app_id text NOT NULL DEFAULT '' CHECK(length(app_id)<=128 AND app_id !~ '[\r\n]'),
 api_base text NOT NULL DEFAULT 'https://api.bot.qq.com' CHECK(length(api_base)>0 AND length(api_base)<=256 AND api_base !~ '[\r\n]'),
 secret_ciphertext bytea NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE community_qq_bot_settings;
