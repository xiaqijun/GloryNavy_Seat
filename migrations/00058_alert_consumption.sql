-- Module: exchange. Prepaid Guoke-coin reservations for time-based Sentry usage.
-- The feature is inert until the host enables the exchange service contract.
-- +goose Up
CREATE TABLE exchange_alert_grants(
    id uuid PRIMARY KEY,
    account_id uuid NOT NULL,
    request_key uuid NOT NULL,
    unit_seconds bigint NOT NULL CHECK(unit_seconds>0),
    unit_price_minor bigint NOT NULL CHECK(unit_price_minor>0),
    reserved_seconds bigint NOT NULL CHECK(reserved_seconds>0),
    reserved_minor bigint NOT NULL CHECK(reserved_minor>0),
    released_seconds bigint NOT NULL DEFAULT 0 CHECK(released_seconds>=0 AND released_seconds<=reserved_seconds),
    settled_seconds bigint NOT NULL DEFAULT 0 CHECK(settled_seconds>=0 AND settled_seconds<=reserved_seconds),
    released_minor bigint NOT NULL DEFAULT 0 CHECK(released_minor>=0 AND released_minor<=reserved_minor),
    settled_minor bigint NOT NULL DEFAULT 0 CHECK(settled_minor>=0 AND settled_minor<=reserved_minor),
    expires_at timestamptz NOT NULL,
    state text NOT NULL DEFAULT 'active' CHECK(state IN('active','closed')),
    created_at timestamptz NOT NULL DEFAULT now(),
    closed_at timestamptz,
    original_account_id uuid,
    UNIQUE(account_id,request_key)
);
CREATE INDEX exchange_alert_grants_account ON exchange_alert_grants(account_id,created_at DESC);
CREATE TABLE exchange_alert_charges(
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    grant_id uuid NOT NULL REFERENCES exchange_alert_grants(id) ON DELETE CASCADE,
    account_id uuid NOT NULL,
    interval_id text NOT NULL,
    started_at timestamptz NOT NULL,
    ended_at timestamptz NOT NULL,
    duration_seconds bigint NOT NULL CHECK(duration_seconds>0),
    coins_minor bigint NOT NULL CHECK(coins_minor>0),
    state text NOT NULL DEFAULT 'reserved' CHECK(state IN('reserved','settled','released','refunded')),
    reserve_key uuid NOT NULL,
    settle_key uuid,
    release_key uuid,
    refund_key uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    decided_at timestamptz,
    original_account_id uuid,
    UNIQUE(grant_id,interval_id),
    CHECK(ended_at>started_at)
);
CREATE INDEX exchange_alert_charges_account ON exchange_alert_charges(account_id,id DESC);
ALTER TABLE exchange_coin_ledger DROP CONSTRAINT exchange_coin_ledger_kind_check;
ALTER TABLE exchange_coin_ledger ADD CONSTRAINT exchange_coin_ledger_kind_check CHECK(kind IN('source','reserve','refund','alert_reserve','alert_release','alert_refund'));
-- +goose Down
ALTER TABLE exchange_coin_ledger DROP CONSTRAINT exchange_coin_ledger_kind_check;
ALTER TABLE exchange_coin_ledger ADD CONSTRAINT exchange_coin_ledger_kind_check CHECK(kind IN('source','reserve','refund'));
DROP TABLE exchange_alert_charges;
DROP TABLE exchange_alert_grants;
