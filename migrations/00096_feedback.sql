-- Module: feedback. Member-owned requests; authors and history remain immutable.
-- +goose Up
CREATE TABLE feedback_tickets (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 account_id uuid NOT NULL REFERENCES identity_users(id),
 original_account_id uuid REFERENCES identity_users(id),
 submitted_by uuid NOT NULL REFERENCES identity_users(id),
 author_name text NOT NULL,
 request_key uuid NOT NULL,
 kind text NOT NULL CHECK(kind IN ('bug','feature')),
 title text NOT NULL CHECK(length(title) BETWEEN 1 AND 120),
 body text NOT NULL CHECK(length(body) BETWEEN 1 AND 6000),
 status text NOT NULL DEFAULT 'submitted' CHECK(status IN ('submitted','in_progress','resolved','closed')),
 revision integer NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(submitted_by,request_key)
);
CREATE INDEX feedback_tickets_owner ON feedback_tickets(account_id,id DESC);
CREATE INDEX feedback_tickets_filter ON feedback_tickets(status,kind,id DESC);
CREATE TABLE feedback_messages (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 ticket_id bigint NOT NULL REFERENCES feedback_tickets(id),
 actor uuid NOT NULL REFERENCES identity_users(id),
 author_name text NOT NULL,
 staff boolean NOT NULL,
 request_key uuid NOT NULL,
 body text NOT NULL CHECK(length(body)<=2000),
 status text NOT NULL DEFAULT '' CHECK(status IN ('','submitted','in_progress','resolved','closed')),
 created_at timestamptz NOT NULL DEFAULT now(),
 CHECK(length(body)>0 OR status<>''),
 UNIQUE(actor,request_key)
);
CREATE INDEX feedback_messages_ticket ON feedback_messages(ticket_id,id);
CREATE INDEX feedback_messages_actor_time ON feedback_messages(actor,created_at);
CREATE INDEX feedback_tickets_author_time ON feedback_tickets(submitted_by,created_at);
CREATE TRIGGER feedback_ticket_active BEFORE INSERT OR UPDATE OF account_id ON feedback_tickets
 FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('account_id');
CREATE TRIGGER feedback_message_active BEFORE INSERT ON feedback_messages
 FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('actor');

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM feedback_tickets) THEN
  RAISE EXCEPTION 'feedback history exists; retain schema and merge participant';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE feedback_messages;
DROP TABLE feedback_tickets;
