-- +goose Up
ALTER TABLE feedback_tickets DROP CONSTRAINT feedback_tickets_status_check;
ALTER TABLE feedback_tickets ADD CONSTRAINT feedback_tickets_status_check CHECK(status IN ('submitted','in_progress','accepted','resolved','closed'));
ALTER TABLE feedback_messages DROP CONSTRAINT feedback_messages_status_check;
ALTER TABLE feedback_messages ADD CONSTRAINT feedback_messages_status_check CHECK(status IN ('','submitted','in_progress','accepted','resolved','closed'));
ALTER TABLE feedback_tickets ADD COLUMN accepted_at timestamptz, ADD COLUMN accepted_by uuid REFERENCES identity_users(id), ADD COLUMN reward_minor bigint CHECK(reward_minor BETWEEN 0 AND 1000000000000);
CREATE TABLE feedback_reward_settings(singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),amount_minor bigint NOT NULL DEFAULT 0 CHECK(amount_minor BETWEEN 0 AND 1000000000000),version bigint NOT NULL DEFAULT 1);
INSERT INTO feedback_reward_settings(singleton) VALUES(true);
CREATE TABLE feedback_reward_config_audit(id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,actor_id uuid NOT NULL REFERENCES identity_users(id),request_key uuid NOT NULL,amount_minor bigint NOT NULL,version bigint NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),UNIQUE(actor_id,request_key));
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM feedback_tickets WHERE accepted_at IS NOT NULL) OR EXISTS(SELECT 1 FROM feedback_reward_config_audit) THEN RAISE EXCEPTION 'Feedback rewards exist; preserve adoption and coin provenance'; END IF;
END $$;
-- +goose StatementEnd
DROP TABLE feedback_reward_config_audit;
DROP TABLE feedback_reward_settings;
ALTER TABLE feedback_tickets DROP COLUMN reward_minor,DROP COLUMN accepted_by,DROP COLUMN accepted_at;
ALTER TABLE feedback_tickets DROP CONSTRAINT feedback_tickets_status_check;
ALTER TABLE feedback_tickets ADD CONSTRAINT feedback_tickets_status_check CHECK(status IN ('submitted','in_progress','resolved','closed'));
ALTER TABLE feedback_messages DROP CONSTRAINT feedback_messages_status_check;
ALTER TABLE feedback_messages ADD CONSTRAINT feedback_messages_status_check CHECK(status IN ('','submitted','in_progress','resolved','closed'));
