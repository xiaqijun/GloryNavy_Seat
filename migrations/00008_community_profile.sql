-- Module: community. Member-supplied details are distinct from trusted confirmations.
-- +goose Up
CREATE TABLE community_profiles (
 user_id uuid PRIMARY KEY REFERENCES identity_users(id),
 qq_number text NOT NULL DEFAULT '' CHECK(qq_number='' OR qq_number ~ '^[1-9][0-9]{4,11}$'),
 kook_name text NOT NULL DEFAULT '' CHECK(char_length(kook_name)<=64),
 version bigint NOT NULL DEFAULT 0 CHECK(version>=0),
 qq_version bigint NOT NULL DEFAULT 0 CHECK(qq_version>=0),
 kook_version bigint NOT NULL DEFAULT 0 CHECK(kook_version>=0),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE community_confirmations (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 user_id uuid NOT NULL REFERENCES community_profiles(user_id),
 platform text NOT NULL CHECK(platform IN ('qq','kook')),
 field_version bigint NOT NULL CHECK(field_version>0),
 source text NOT NULL CHECK(length(source)>0),
 actor text NOT NULL CHECK(length(actor)>0),
 event_id text NOT NULL CHECK(length(event_id)>0),
 confirmed_at timestamptz NOT NULL DEFAULT now(),
 invalidated_at timestamptz,
 UNIQUE(source,event_id)
);
CREATE UNIQUE INDEX community_current_confirmation ON community_confirmations(user_id,platform) WHERE invalidated_at IS NULL;
CREATE TABLE community_profile_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 user_id uuid NOT NULL REFERENCES community_profiles(user_id),
 profile_version bigint NOT NULL,
 changed_platforms text[] NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
-- +goose Down
DROP TABLE community_profile_events;
DROP TABLE community_confirmations;
DROP TABLE community_profiles;
