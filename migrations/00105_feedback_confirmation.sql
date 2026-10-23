-- +goose Up
ALTER TABLE feedback_tickets DROP CONSTRAINT feedback_tickets_status_check;
ALTER TABLE feedback_tickets ADD CONSTRAINT feedback_tickets_status_check CHECK(status IN ('submitted','accepted','in_progress','awaiting_confirmation','resolved','closed'));
ALTER TABLE feedback_messages DROP CONSTRAINT feedback_messages_status_check;
ALTER TABLE feedback_messages ADD CONSTRAINT feedback_messages_status_check CHECK(status IN ('','submitted','accepted','in_progress','awaiting_confirmation','resolved','closed'));
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM feedback_tickets WHERE status='awaiting_confirmation') OR EXISTS(SELECT 1 FROM feedback_messages WHERE status='awaiting_confirmation') THEN RAISE EXCEPTION 'Feedback confirmation history exists; retain workflow states'; END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE feedback_tickets DROP CONSTRAINT feedback_tickets_status_check;
ALTER TABLE feedback_tickets ADD CONSTRAINT feedback_tickets_status_check CHECK(status IN ('submitted','accepted','in_progress','resolved','closed'));
ALTER TABLE feedback_messages DROP CONSTRAINT feedback_messages_status_check;
ALTER TABLE feedback_messages ADD CONSTRAINT feedback_messages_status_check CHECK(status IN ('','submitted','accepted','in_progress','resolved','closed'));
