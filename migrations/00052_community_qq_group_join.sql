-- Module: community. Official QQ group admission applications and scoped bindings.
-- +goose Up
CREATE TABLE community_qq_group_applications (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 user_id uuid NOT NULL REFERENCES community_profiles(user_id),
 group_openid text NOT NULL CHECK(length(group_openid)>0 AND length(group_openid)<=256),
 qq_number text NOT NULL CHECK(qq_number ~ '^[1-9][0-9]{4,11}$'),
 qq_version bigint NOT NULL CHECK(qq_version>0),
 code_hash bytea NOT NULL CHECK(octet_length(code_hash)=32),
 status text NOT NULL CHECK(status IN ('pending','approving','approved','bound','rejected','expired','failed')),
 member_openid text,
 join_request_id text,
 failure_reason text,
 created_at timestamptz NOT NULL DEFAULT now(),
 expires_at timestamptz NOT NULL,
 approved_at timestamptz,
 bound_at timestamptz,
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX community_qq_group_applications_user ON community_qq_group_applications(user_id,created_at DESC);
CREATE INDEX community_qq_group_applications_pending ON community_qq_group_applications(group_openid,status,created_at)
 WHERE status IN ('pending','approving','approved');
CREATE UNIQUE INDEX community_qq_group_applications_active_user_group
 ON community_qq_group_applications(user_id,group_openid)
 WHERE status IN ('pending','approving','approved','bound');
CREATE TRIGGER community_qq_group_applications_active_account BEFORE INSERT OR UPDATE
 ON community_qq_group_applications FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('user_id');

CREATE TABLE community_qq_group_bindings (
 user_id uuid NOT NULL REFERENCES community_profiles(user_id),
 group_openid text NOT NULL CHECK(length(group_openid)>0 AND length(group_openid)<=256),
 member_openid text NOT NULL CHECK(length(member_openid)>0 AND length(member_openid)<=256),
 qq_number text NOT NULL CHECK(qq_number ~ '^[1-9][0-9]{4,11}$'),
 application_id bigint NOT NULL REFERENCES community_qq_group_applications(id),
 bound_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(group_openid,member_openid),
 UNIQUE(user_id,group_openid)
);
CREATE TRIGGER community_qq_group_bindings_active_account BEFORE INSERT OR UPDATE
 ON community_qq_group_bindings FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('user_id');

-- +goose Down
DROP TRIGGER community_qq_group_bindings_active_account ON community_qq_group_bindings;
DROP TRIGGER community_qq_group_applications_active_account ON community_qq_group_applications;
DROP TABLE community_qq_group_bindings;
DROP TABLE community_qq_group_applications;
