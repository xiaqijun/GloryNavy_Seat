-- +goose Up
CREATE TABLE reminder_email_settings (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 enabled boolean NOT NULL DEFAULT false,
 host text NOT NULL DEFAULT '', port integer NOT NULL DEFAULT 465,
 implicit_tls boolean NOT NULL DEFAULT true,
 username text NOT NULL DEFAULT '', sender text NOT NULL DEFAULT '',
 secret_ciphertext bytea NOT NULL DEFAULT '',
 revision bigint NOT NULL DEFAULT 1,
 events text[] NOT NULL DEFAULT '{}',
 event_started jsonb NOT NULL DEFAULT '{}',
 CHECK(port BETWEEN 1 AND 65535),
 CHECK(NOT enabled OR (host<>'' AND username<>'' AND sender<>'' AND octet_length(secret_ciphertext)>0 AND cardinality(events)>0))
);
INSERT INTO reminder_email_settings(singleton) VALUES(true);

CREATE TABLE reminder_email_accounts (
 account_id uuid PRIMARY KEY REFERENCES identity_users(id),
 email text NOT NULL DEFAULT '', verified boolean NOT NULL DEFAULT false,
 enabled boolean NOT NULL DEFAULT false, revision bigint NOT NULL DEFAULT 1,
 pending_email text NOT NULL DEFAULT '', code_hash bytea NOT NULL DEFAULT '',
 code_salt bytea NOT NULL DEFAULT '', code_expires timestamptz,
 code_attempts integer NOT NULL DEFAULT 0, last_code_at timestamptz,
 CHECK(NOT enabled OR verified),
 CHECK(NOT verified OR email<>'')
);

CREATE TABLE reminder_email_deliveries (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 account_id uuid NOT NULL REFERENCES identity_users(id),
 original_account_id uuid REFERENCES identity_users(id),
 recipient text NOT NULL, event text NOT NULL, notice_count integer NOT NULL CHECK(notice_count>0),
 state text NOT NULL CHECK(state IN ('sending','sent','failed','unknown','retry_requested')),
 reason text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(), finished_at timestamptz
);
CREATE INDEX reminder_email_deliveries_account ON reminder_email_deliveries(account_id,created_at DESC);
CREATE TABLE reminder_email_receipts (
 account_id uuid NOT NULL REFERENCES identity_users(id), reference text NOT NULL, event text NOT NULL,
 active boolean NOT NULL DEFAULT true, generation bigint NOT NULL DEFAULT 1,
 attempted_generation bigint NOT NULL DEFAULT 0, delivery_id uuid,
 updated_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(account_id,reference)
);
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM reminder_email_accounts) OR EXISTS(SELECT 1 FROM reminder_email_deliveries)
 OR EXISTS(SELECT 1 FROM reminder_email_receipts) OR EXISTS(SELECT 1 FROM reminder_email_settings WHERE revision<>1) THEN
  RAISE EXCEPTION 'Email reminders are in use; preserve settings and delivery receipts';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE reminder_email_receipts;
DROP TABLE reminder_email_deliveries;
DROP TABLE reminder_email_accounts;
DROP TABLE reminder_email_settings;
