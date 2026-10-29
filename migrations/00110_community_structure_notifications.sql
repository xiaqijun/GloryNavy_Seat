-- Module: community. Official QQ structure notifications are independently configured.
-- +goose Up
CREATE TABLE community_qq_structure_watch (
 id boolean PRIMARY KEY DEFAULT true CHECK(id), enabled boolean NOT NULL DEFAULT false,
 app_id text NOT NULL DEFAULT '', group_openid text NOT NULL DEFAULT '', corporation_id bigint NOT NULL DEFAULT 0,
 owner_id uuid REFERENCES identity_users(id), revision bigint NOT NULL DEFAULT 1 CHECK(revision > 0),
 updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO community_qq_structure_watch(id) VALUES(true);
CREATE TABLE community_qq_structure_observations (
 watch_revision bigint NOT NULL, corporation_id bigint NOT NULL, structure_id bigint NOT NULL, observed_at timestamptz NOT NULL,
 state text NOT NULL, services jsonb NOT NULL, PRIMARY KEY(watch_revision,corporation_id,structure_id)
);
CREATE TABLE community_qq_structure_notices (
 id bigserial PRIMARY KEY, watch_revision bigint NOT NULL, reference text NOT NULL, kind text NOT NULL CHECK(kind IN ('change','fuel','unanchor')),
 corporation_id bigint NOT NULL, structure_id bigint NOT NULL, body text NOT NULL,
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','sending','sent','rejected','unknown','obsolete')),
 error_code integer, created_at timestamptz NOT NULL DEFAULT now(), attempted_at timestamptz,
 UNIQUE(watch_revision,reference)
);
CREATE INDEX community_qq_structure_pending ON community_qq_structure_notices(watch_revision,id) WHERE status='pending';

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM community_qq_structure_watch WHERE enabled OR revision > 1)
 OR EXISTS(SELECT 1 FROM community_qq_structure_notices)
 OR EXISTS(SELECT 1 FROM community_qq_structure_observations)
 THEN RAISE EXCEPTION 'QQ structure notifications are configured or have history; retain their delivery evidence'; END IF;
END $$;
-- +goose StatementEnd
DROP TABLE community_qq_structure_notices;
DROP TABLE community_qq_structure_observations;
DROP TABLE community_qq_structure_watch;
