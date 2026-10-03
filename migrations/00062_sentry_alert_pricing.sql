-- Module: sentry. Versioned administrator-managed pricing for alert usage.
-- The administrator-controlled charging switch is added by Goose 64.
-- +goose Up
CREATE TABLE sentry_alert_pricing (
    id smallint PRIMARY KEY CHECK (id = 1),
    price_version text NOT NULL CHECK (char_length(price_version) BETWEEN 1 AND 80 AND price_version !~ '[\r\n]'),
    unit_seconds bigint NOT NULL CHECK (unit_seconds > 0 AND unit_seconds <= 86400),
    unit_price_minor bigint NOT NULL CHECK (unit_price_minor > 0 AND unit_price_minor <= 1000000000000),
    max_grant_seconds bigint NOT NULL CHECK (max_grant_seconds > 0 AND max_grant_seconds <= 2678400),
    grant_ttl_seconds bigint NOT NULL CHECK (grant_ttl_seconds >= 60 AND grant_ttl_seconds <= 2678400),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_by uuid NOT NULL REFERENCES identity_users(id),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sentry_alert_pricing_audit (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_id uuid NOT NULL REFERENCES identity_users(id),
    previous_version bigint NOT NULL CHECK (previous_version >= 0),
    version bigint NOT NULL CHECK (version > 0),
    before_snapshot jsonb NOT NULL,
    after_snapshot jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sentry_alert_pricing_audit_created ON sentry_alert_pricing_audit(created_at DESC, id DESC);
CREATE TRIGGER sentry_alert_pricing_audit_active_actor BEFORE INSERT ON sentry_alert_pricing_audit
    FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('actor_id');

-- +goose Down
DROP TRIGGER sentry_alert_pricing_audit_active_actor ON sentry_alert_pricing_audit;
DROP TABLE sentry_alert_pricing_audit;
DROP TABLE sentry_alert_pricing;
