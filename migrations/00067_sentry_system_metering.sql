-- Module: sentry/exchange. Attribute monitoring rewards and warning charges
-- to the solar system whose primary node produced the interval.
-- +goose Up
ALTER TABLE sentry_monitor_rewards
    ADD COLUMN system_id text NOT NULL DEFAULT '',
    ADD COLUMN primary_generation bigint NOT NULL DEFAULT 0;

ALTER TABLE sentry_monitor_remainders
    ADD COLUMN system_id text NOT NULL DEFAULT '';

ALTER TABLE sentry_monitor_remainders
    DROP CONSTRAINT sentry_monitor_remainders_pkey;

ALTER TABLE sentry_monitor_remainders
    ADD PRIMARY KEY (account_id, system_id);

CREATE INDEX sentry_monitor_rewards_system_id_interval
    ON sentry_monitor_rewards(system_id, started_at, ended_at)
    WHERE state='rewarded';

ALTER TABLE exchange_alert_grants
    ADD COLUMN system_id text NOT NULL DEFAULT '';

ALTER TABLE exchange_alert_charges
    ADD COLUMN system_id text NOT NULL DEFAULT '';

CREATE INDEX exchange_alert_grants_system_account
    ON exchange_alert_grants(system_id, account_id, created_at DESC);

CREATE INDEX exchange_alert_charges_system_account
    ON exchange_alert_charges(system_id, account_id, id DESC);

-- +goose Down
DROP INDEX exchange_alert_charges_system_account;
DROP INDEX exchange_alert_grants_system_account;
ALTER TABLE exchange_alert_charges DROP COLUMN system_id;
ALTER TABLE exchange_alert_grants DROP COLUMN system_id;
DROP INDEX sentry_monitor_rewards_system_id_interval;
ALTER TABLE sentry_monitor_remainders DROP CONSTRAINT sentry_monitor_remainders_pkey;
ALTER TABLE sentry_monitor_remainders DROP COLUMN system_id;
ALTER TABLE sentry_monitor_remainders ADD PRIMARY KEY (account_id);
ALTER TABLE sentry_monitor_rewards DROP COLUMN primary_generation;
ALTER TABLE sentry_monitor_rewards DROP COLUMN system_id;
