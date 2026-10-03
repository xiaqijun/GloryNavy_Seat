-- Module: sentry. Hourly prices and immutable server-confirmed monitor rewards.
-- +goose Up
CREATE TABLE sentry_time_pricing (
    version bigint PRIMARY KEY CHECK (version > 0),
    alert_hourly_price_minor bigint NOT NULL CHECK (alert_hourly_price_minor BETWEEN 1 AND 1000000000000),
    monitor_hourly_reward_minor bigint NOT NULL CHECK (monitor_hourly_reward_minor BETWEEN 0 AND 1000000000000),
    updated_by uuid NOT NULL REFERENCES identity_users(id),
    effective_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER sentry_time_pricing_active_actor BEFORE INSERT ON sentry_time_pricing
    FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('updated_by');

CREATE TABLE sentry_monitor_rewards (
    contribution_id text PRIMARY KEY CHECK (char_length(contribution_id) BETWEEN 1 AND 200),
    account_id uuid REFERENCES identity_users(id),
    original_account_id uuid REFERENCES identity_users(id),
    remote_key_id text NOT NULL,
    client_id text NOT NULL,
    system_name text NOT NULL,
    started_at timestamptz NOT NULL,
    ended_at timestamptz NOT NULL CHECK (ended_at > started_at),
    duration_seconds bigint NOT NULL CHECK (duration_seconds BETWEEN 1 AND 86400),
    state text NOT NULL CHECK (state IN ('rewarded','excluded')),
    numerator bigint NOT NULL CHECK (numerator >= 0),
    coins_minor bigint NOT NULL CHECK (coins_minor >= 0),
    fingerprint text NOT NULL,
    evidence jsonb NOT NULL,
    price_snapshot jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sentry_monitor_rewards_account ON sentry_monitor_rewards(account_id, created_at DESC);
CREATE INDEX sentry_monitor_rewards_system_interval ON sentry_monitor_rewards(lower(system_name),started_at,ended_at) WHERE state='rewarded';
CREATE TABLE sentry_monitor_remainders (
    account_id uuid PRIMARY KEY REFERENCES identity_users(id),
    numerator bigint NOT NULL DEFAULT 0 CHECK (numerator >= 0)
);
CREATE TABLE sentry_monitor_reconcile_state (
    id smallint PRIMARY KEY CHECK (id=1),
    cursor text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
-- Stop sentry.monitor-reconcile.v1 jobs before rolling back this schema.
DROP TABLE sentry_monitor_reconcile_state;
DROP TABLE sentry_monitor_remainders;
DROP TABLE sentry_monitor_rewards;
DROP TRIGGER sentry_time_pricing_active_actor ON sentry_time_pricing;
DROP TABLE sentry_time_pricing;
