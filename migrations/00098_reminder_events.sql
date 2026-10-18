-- +goose Up
ALTER TABLE reminder_settings ADD COLUMN events text[] NOT NULL DEFAULT '{approval_pending}';
ALTER TABLE reminder_settings ADD COLUMN event_started jsonb NOT NULL DEFAULT '{}';
ALTER TABLE reminder_settings ADD CONSTRAINT reminder_event_selection CHECK(events <@ ARRAY['approval_pending','welfare_result','exchange_result','loan_result','contract_pending','guarantee_pending','loan_funding','loan_due','loan_overdue','pap_requirement','pap_change','coin_change','authorization_expired','structure_alert','sync_anomaly','sentry_alert','feedback_update','feedback_pending','collateral_pending','contribution_result']::text[]);
ALTER TABLE reminder_deliveries DROP CONSTRAINT reminder_deliveries_event_check;
ALTER TABLE reminder_deliveries ADD CONSTRAINT reminder_deliveries_event_check CHECK(event IN ('approval_pending','welfare_result','exchange_result','loan_result','contract_pending','guarantee_pending','loan_funding','loan_due','loan_overdue','pap_requirement','pap_change','coin_change','authorization_expired','structure_alert','sync_anomaly','sentry_alert','feedback_update','feedback_pending','collateral_pending','contribution_result'));
ALTER TABLE reminder_receipts ADD COLUMN event text NOT NULL DEFAULT 'approval_pending';
CREATE INDEX reminder_receipts_event ON reminder_receipts(account_id,event) WHERE active;
CREATE INDEX reminder_deliveries_account_event ON reminder_deliveries(account_id,event,created_at DESC);
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM reminder_deliveries WHERE event<>'approval_pending') OR EXISTS(SELECT 1 FROM reminder_receipts WHERE event<>'approval_pending') OR EXISTS(SELECT 1 FROM reminder_settings WHERE events<>'{approval_pending}'::text[]) THEN
  RAISE EXCEPTION 'New reminder events are in use; preserve settings and receipts';
 END IF;
END $$;
-- +goose StatementEnd
DROP INDEX reminder_deliveries_account_event;
DROP INDEX reminder_receipts_event;
ALTER TABLE reminder_receipts DROP COLUMN event;
ALTER TABLE reminder_deliveries DROP CONSTRAINT reminder_deliveries_event_check;
ALTER TABLE reminder_deliveries ADD CONSTRAINT reminder_deliveries_event_check CHECK(event='approval_pending');
ALTER TABLE reminder_settings DROP CONSTRAINT reminder_event_selection;
ALTER TABLE reminder_settings DROP COLUMN event_started, DROP COLUMN events;
