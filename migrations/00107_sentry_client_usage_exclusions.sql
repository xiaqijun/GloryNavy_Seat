-- Module: sentry. Preserve raw evidence intentionally excluded from charging.
-- +goose Up
CREATE TABLE sentry_client_usage_exclusions (
    usage_id text PRIMARY KEY CHECK (char_length(usage_id) BETWEEN 1 AND 200),
    account_id uuid NOT NULL REFERENCES identity_users(id),
    original_account_id uuid REFERENCES identity_users(id),
    system_id text NOT NULL,
    started_at timestamptz NOT NULL,
    ended_at timestamptz NOT NULL CHECK (ended_at > started_at),
    duration_seconds bigint NOT NULL CHECK (duration_seconds > 0),
    reason text NOT NULL CHECK (reason IN ('before_latest_enable','crosses_latest_enable')),
    enable_audit_id bigint NOT NULL REFERENCES sentry_alert_pricing_audit(id),
    enabled_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sentry_client_usage_exclusions_account ON sentry_client_usage_exclusions(account_id,created_at);
CREATE TRIGGER sentry_client_usage_exclusions_active_account BEFORE INSERT OR UPDATE ON sentry_client_usage_exclusions
    FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('account_id');

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM sentry_client_usage_exclusions) THEN
        RAISE EXCEPTION 'Client usage exclusions exist; preserve evidence and fix forward';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE sentry_client_usage_exclusions;
