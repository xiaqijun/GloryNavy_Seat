-- Modules: identity and eve. Explicit multi-character ownership and SSO intent.
-- +goose Up
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS (SELECT user_id FROM identity_characters GROUP BY user_id HAVING count(*) > 1) THEN
    RAISE EXCEPTION 'Multiple characters already exist for a user; select a main explicitly before upgrading';
  END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE identity_users ADD COLUMN main_character_id bigint;
UPDATE identity_users u SET main_character_id=c.character_id FROM identity_characters c WHERE c.user_id=u.id;
ALTER TABLE identity_users ADD CONSTRAINT identity_main_owned FOREIGN KEY(id,main_character_id)
  REFERENCES identity_characters(user_id,character_id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE identity_character_events (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id uuid NOT NULL,
  character_id bigint NOT NULL,
  action text NOT NULL CHECK(action IN ('linked','main_changed','unlinked','ownership_blocked')),
  created_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE eve_login_flows ADD COLUMN intent text NOT NULL DEFAULT 'login' CHECK(intent IN ('login','link','reauthorize'));
ALTER TABLE eve_login_flows ADD COLUMN target_user_id uuid;
ALTER TABLE eve_login_flows ADD COLUMN expected_character_id bigint;
ALTER TABLE eve_login_flows ADD CONSTRAINT eve_flow_intent CHECK (
 (intent='login' AND target_user_id IS NULL AND expected_character_id IS NULL) OR
 (intent='link' AND target_user_id IS NOT NULL AND expected_character_id IS NULL) OR
 (intent='reauthorize' AND target_user_id IS NOT NULL AND expected_character_id > 0 AND expected_character_id IS NOT NULL)
);
-- +goose Down
ALTER TABLE eve_login_flows DROP CONSTRAINT eve_flow_intent;
ALTER TABLE eve_login_flows DROP COLUMN expected_character_id, DROP COLUMN target_user_id, DROP COLUMN intent;
DROP TABLE identity_character_events;
ALTER TABLE identity_users DROP CONSTRAINT identity_main_owned;
ALTER TABLE identity_users DROP COLUMN main_character_id;
