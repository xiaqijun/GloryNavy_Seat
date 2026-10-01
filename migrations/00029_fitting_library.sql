-- +goose Up
CREATE TABLE fittings_library (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 corporation_id bigint NOT NULL CHECK(corporation_id>0),
 name text NOT NULL CHECK(char_length(name) BETWEEN 1 AND 50),
 description text NOT NULL DEFAULT '' CHECK(char_length(description)<=500),
 fit jsonb NOT NULL,
 version bigint NOT NULL DEFAULT 1,
 created_by uuid NOT NULL,
 request_key uuid NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(created_by,request_key)
);
CREATE INDEX fittings_library_corporation ON fittings_library(corporation_id,id DESC);
CREATE TABLE fittings_library_audit (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 library_id bigint NOT NULL,
 actor_id uuid NOT NULL,
 action text NOT NULL CHECK(action IN ('import','update','delete')),
 before_value jsonb,
 after_value jsonb,
 created_at timestamptz NOT NULL DEFAULT now()
);
-- A dispatch is never automatically repeated after an ambiguous ESI write.
CREATE TABLE fittings_game_saves (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 account_id uuid NOT NULL,
 character_id bigint NOT NULL,
 library_id bigint NOT NULL,
 library_version bigint NOT NULL,
 request_key uuid NOT NULL,
 state text NOT NULL CHECK(state IN ('sending','saved','failed','unknown')),
 fitting_id bigint,
 reason text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(account_id,request_key),
 UNIQUE(account_id,character_id,library_id,library_version)
);
-- +goose Down
DROP TABLE fittings_game_saves;
DROP TABLE fittings_library_audit;
DROP TABLE fittings_library;
