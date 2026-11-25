-- +goose Up
ALTER TABLE community_qq_group_applications DROP CONSTRAINT community_qq_group_applications_status_check;
ALTER TABLE community_qq_group_applications ADD CONSTRAINT community_qq_group_applications_status_check
    CHECK (status IN ('pending','waiting_membership','approving','approved','bound','rejected','expired','failed'));
ALTER TABLE community_qq_group_applications ADD CONSTRAINT community_qq_group_waiting_has_request
    CHECK (status<>'waiting_membership' OR (COALESCE(member_openid,'')<>'' AND COALESCE(join_request_id,'')<>''));

DROP INDEX community_qq_group_applications_active_user_group;
CREATE UNIQUE INDEX community_qq_group_applications_active_user_group
    ON community_qq_group_applications(user_id,group_openid)
    WHERE status IN ('pending','waiting_membership','approving','approved','bound');

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM community_qq_group_applications WHERE status='waiting_membership') THEN
        RAISE EXCEPTION 'QQ group applications are waiting for corporation membership';
    END IF;
END $$;
-- +goose StatementEnd

DROP INDEX community_qq_group_applications_active_user_group;
CREATE UNIQUE INDEX community_qq_group_applications_active_user_group
    ON community_qq_group_applications(user_id,group_openid)
    WHERE status IN ('pending','approving','approved','bound');

ALTER TABLE community_qq_group_applications DROP CONSTRAINT community_qq_group_applications_status_check;
ALTER TABLE community_qq_group_applications DROP CONSTRAINT community_qq_group_waiting_has_request;
ALTER TABLE community_qq_group_applications ADD CONSTRAINT community_qq_group_applications_status_check
    CHECK (status IN ('pending','approving','approved','bound','rejected','expired','failed'));
