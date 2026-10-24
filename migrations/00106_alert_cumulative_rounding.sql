-- Module: exchange. Start cumulative client-usage rounding from new evidence.
-- Historical grants, charges, and coin entries are deliberately untouched.
-- +goose Up
ALTER TABLE exchange_alert_grants ADD COLUMN cumulative_rounding boolean NOT NULL DEFAULT false;
ALTER TABLE exchange_alert_grants DROP CONSTRAINT exchange_alert_grants_reserved_minor_check;
ALTER TABLE exchange_alert_grants ADD CONSTRAINT exchange_alert_grants_reserved_minor_check CHECK(reserved_minor>=0);
ALTER TABLE exchange_alert_charges DROP CONSTRAINT exchange_alert_charges_coins_minor_check;
ALTER TABLE exchange_alert_charges ADD CONSTRAINT exchange_alert_charges_coins_minor_check CHECK(coins_minor>=0);

CREATE TABLE exchange_alert_rounding (
    account_id uuid NOT NULL REFERENCES identity_users(id),
    system_id text NOT NULL,
    price_version text NOT NULL,
    unit_seconds bigint NOT NULL CHECK(unit_seconds>0),
    unit_price_minor bigint NOT NULL CHECK(unit_price_minor>0),
    credit_numerator bigint NOT NULL DEFAULT 0 CHECK(credit_numerator>=0),
    original_account_id uuid REFERENCES identity_users(id),
    PRIMARY KEY(account_id,system_id,price_version,unit_seconds,unit_price_minor)
);
CREATE TRIGGER exchange_alert_rounding_active_account BEFORE INSERT OR UPDATE ON exchange_alert_rounding
    FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('account_id');

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM exchange_alert_rounding)
       OR EXISTS(SELECT 1 FROM exchange_alert_grants WHERE cumulative_rounding)
    THEN RAISE EXCEPTION 'Cumulative alert billing exists; retain rounding credit and use a compatible release'; END IF;
END $$;
-- +goose StatementEnd
DROP TABLE exchange_alert_rounding;
ALTER TABLE exchange_alert_grants DROP COLUMN cumulative_rounding;
ALTER TABLE exchange_alert_grants DROP CONSTRAINT exchange_alert_grants_reserved_minor_check;
ALTER TABLE exchange_alert_grants ADD CONSTRAINT exchange_alert_grants_reserved_minor_check CHECK(reserved_minor>0);
ALTER TABLE exchange_alert_charges DROP CONSTRAINT exchange_alert_charges_coins_minor_check;
ALTER TABLE exchange_alert_charges ADD CONSTRAINT exchange_alert_charges_coins_minor_check CHECK(coins_minor>0);
