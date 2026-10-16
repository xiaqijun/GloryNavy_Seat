-- Module: identity. Weekly, administrator-configured character cleanup.
-- +goose Up
CREATE TABLE identity_corporation_cleanup (
 id boolean PRIMARY KEY DEFAULT true CHECK(id),
 enabled boolean NOT NULL DEFAULT false,
 corporation_ids bigint[] NOT NULL DEFAULT '{}',
 version bigint NOT NULL DEFAULT 1,
 next_run_at timestamptz,
 run_started_at timestamptz,
 after_user_id uuid,
 removed_count integer NOT NULL DEFAULT 0,
 skipped_count integer NOT NULL DEFAULT 0,
 last_run_at timestamptz,
 last_removed_count integer NOT NULL DEFAULT 0,
 last_skipped_count integer NOT NULL DEFAULT 0,
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(NOT enabled OR cardinality(corporation_ids)>0)
);
INSERT INTO identity_corporation_cleanup(id) VALUES(true);
CREATE TABLE identity_cleanup_removed (
 character_id bigint PRIMARY KEY,
 user_id uuid NOT NULL REFERENCES identity_users(id),
 original_user_id uuid NOT NULL,
 owner_hash bytea NOT NULL CHECK(octet_length(owner_hash)=32),
 name text NOT NULL,
 corporation_id bigint NOT NULL,
 removed_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE identity_cleanup_audit (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 actor text NOT NULL,
 action text NOT NULL CHECK(action IN ('configured','removed','completed')),
 detail jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM identity_cleanup_removed) THEN
  RAISE EXCEPTION 'Cannot remove preserved character ownership after automatic cleanup';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE identity_cleanup_audit;
DROP TABLE identity_cleanup_removed;
DROP TABLE identity_corporation_cleanup;
