-- Official QQ is the only active transport. Preserve historical OneBot evidence.
-- +goose Up
ALTER TABLE community_qq_bot_settings ADD COLUMN enabled boolean NOT NULL DEFAULT false;
ALTER TABLE community_qq_bot_settings ADD COLUMN revision bigint NOT NULL DEFAULT 1 CHECK(revision>0);
INSERT INTO community_qq_bot_settings(id) VALUES(true) ON CONFLICT(id) DO NOTHING;
UPDATE community_onebot_settings SET enabled=false,revision=revision+1,updated_at=now() WHERE enabled;
UPDATE community_onebot_groups SET notify_approvals=false WHERE notify_approvals;
UPDATE community_confirmations SET invalidated_at=now() WHERE platform='qq' AND source='onebot' AND invalidated_at IS NULL;
UPDATE community_qq_link_challenges SET used_at=now() WHERE used_at IS NULL;

-- +goose Down
-- Empty, unused test/development installations may drop the new fields.
-- Configured instances must preserve transport and identity boundaries.
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM community_qq_bot_settings WHERE enabled OR revision>1 OR app_id<>'' OR octet_length(secret_ciphertext)>0)
 OR EXISTS(SELECT 1 FROM community_qq_bindings)
 THEN RAISE EXCEPTION 'Official QQ transport configured; retain settings, identity isolation and historical receipts'; END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE community_qq_bot_settings DROP COLUMN revision;
ALTER TABLE community_qq_bot_settings DROP COLUMN enabled;
