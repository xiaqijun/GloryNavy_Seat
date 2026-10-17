-- +goose Up
ALTER TABLE community_onebot_groups ADD COLUMN notify_approvals boolean NOT NULL DEFAULT false;
-- Notification receipts only: approval state and decisions remain in their source modules.
-- No group FK: saving the group whitelist replaces its rows without erasing deduplication.
CREATE TABLE community_approval_notices (
 group_id text NOT NULL,
 user_id uuid NOT NULL REFERENCES community_profiles(user_id) ON DELETE CASCADE,
 reference text NOT NULL,
 active boolean NOT NULL DEFAULT true,
 generation bigint NOT NULL DEFAULT 1,
 attempted_generation bigint NOT NULL DEFAULT 0,
 batch_id uuid,
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(group_id,user_id,reference)
);
CREATE TABLE community_approval_deliveries (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 group_id text NOT NULL,
 state text NOT NULL CHECK(state IN ('sending','sent','unknown')),
 recipient_count integer NOT NULL CHECK(recipient_count>0),
 notice_count integer NOT NULL CHECK(notice_count>0),
 message_id text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 finished_at timestamptz
);
CREATE INDEX community_approval_deliveries_group_time ON community_approval_deliveries(group_id,created_at DESC);
CREATE INDEX community_approval_notices_user ON community_approval_notices(user_id);
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM community_approval_deliveries) OR EXISTS(SELECT 1 FROM community_onebot_groups WHERE notify_approvals) THEN
  RAISE EXCEPTION 'Approval notifications are in use; disable reminders and retain receipts before a reviewed rollback';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE community_approval_deliveries;
DROP TABLE community_approval_notices;
ALTER TABLE community_onebot_groups DROP COLUMN notify_approvals;
