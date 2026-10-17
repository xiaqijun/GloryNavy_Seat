-- +goose Up
CREATE TABLE reminder_settings (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 enabled boolean NOT NULL DEFAULT false,
 sender_id bigint NOT NULL DEFAULT 0,
 sender_account uuid REFERENCES identity_users(id),
 sender_owner bytea NOT NULL DEFAULT '',
 revision bigint NOT NULL DEFAULT 1,
 CHECK(NOT enabled OR (sender_id>0 AND sender_account IS NOT NULL AND octet_length(sender_owner)>0))
);
INSERT INTO reminder_settings(singleton) VALUES(true);
CREATE TABLE reminder_audit (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 actor uuid NOT NULL REFERENCES identity_users(id),
 action text NOT NULL,
 revision bigint NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE reminder_deliveries (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 account_id uuid NOT NULL REFERENCES identity_users(id),
 original_account_id uuid REFERENCES identity_users(id),
 channel text NOT NULL DEFAULT 'eve_mail' CHECK(channel='eve_mail'),
 event text NOT NULL DEFAULT 'approval_pending' CHECK(event='approval_pending'),
 sender_id bigint NOT NULL,
 recipient_id bigint NOT NULL,
 recipient_name text NOT NULL,
 notice_count integer NOT NULL CHECK(notice_count>0),
 state text NOT NULL CHECK(state IN ('sending','sent','failed','unknown','retry_requested')),
 reason text NOT NULL DEFAULT '',
 message_id bigint NOT NULL DEFAULT 0,
 created_at timestamptz NOT NULL DEFAULT now(),
 finished_at timestamptz
);
CREATE INDEX reminder_deliveries_time ON reminder_deliveries(created_at DESC,id DESC);
CREATE INDEX reminder_deliveries_account ON reminder_deliveries(account_id);
CREATE TABLE reminder_receipts (
 account_id uuid NOT NULL REFERENCES identity_users(id),
 reference text NOT NULL,
 active boolean NOT NULL DEFAULT true,
 generation bigint NOT NULL DEFAULT 1,
 attempted_generation bigint NOT NULL DEFAULT 0,
 delivery_id uuid,
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(account_id,reference)
);
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM reminder_deliveries) OR EXISTS(SELECT 1 FROM reminder_audit) OR EXISTS(SELECT 1 FROM reminder_settings WHERE enabled) THEN
  RAISE EXCEPTION 'Reminders are in use; preserve delivery receipts and review rollback first';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE reminder_receipts;
DROP TABLE reminder_deliveries;
DROP TABLE reminder_audit;
DROP TABLE reminder_settings;
