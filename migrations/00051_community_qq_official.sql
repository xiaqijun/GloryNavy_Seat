-- Module: community. Official QQ bot binding uses a short-lived challenge.
-- +goose Up
CREATE TABLE community_qq_link_challenges (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 user_id uuid NOT NULL REFERENCES community_profiles(user_id),
 code_hash bytea NOT NULL CHECK(octet_length(code_hash)=32),
 qq_version bigint NOT NULL CHECK(qq_version>0),
 expires_at timestamptz NOT NULL,
 used_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX community_qq_link_challenges_active ON community_qq_link_challenges(user_id,expires_at) WHERE used_at IS NULL;
CREATE TABLE community_qq_bindings (
 user_id uuid PRIMARY KEY REFERENCES community_profiles(user_id),
 openid text NOT NULL UNIQUE CHECK(length(openid)>0),
 qq_version bigint NOT NULL CHECK(qq_version>0),
 bound_at timestamptz NOT NULL DEFAULT now()
);
-- +goose Down
DROP TABLE community_qq_bindings;
DROP TABLE community_qq_link_challenges;
