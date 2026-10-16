-- Module: loan. Periodic credit snapshots, asset-supported capacity and
-- immutable collateral valuation inputs.
-- +goose Up
ALTER TABLE loan_credit_policy
    ADD COLUMN asset_credit_ltv_bps integer NOT NULL DEFAULT 1000 CHECK (asset_credit_ltv_bps BETWEEN 0 AND 10000);

ALTER TABLE loan_credit_policy
    ALTER COLUMN min_income_samples SET DEFAULT 1,
    ALTER COLUMN insufficient_income_policy SET DEFAULT 'zero';

ALTER TABLE loan_credit_evaluations
    ADD COLUMN guarantee_limit_minor bigint NOT NULL DEFAULT 0 CHECK (guarantee_limit_minor >= 0),
    ADD COLUMN asset_support_limit_minor bigint NOT NULL DEFAULT 0 CHECK (asset_support_limit_minor >= 0);

CREATE INDEX loan_credit_evaluations_latest ON loan_credit_evaluations(account_id, evaluated_at DESC, id DESC);

-- Existing installations use a conservative, evidence-first default. Do not
-- rewrite historical evaluations; only future periodic snapshots use it.
UPDATE loan_credit_policy
SET asset_credit_ltv_bps = 1000,
    min_income_samples = 1,
    insufficient_income_policy = 'zero'
WHERE singleton;

-- +goose Down
DROP INDEX loan_credit_evaluations_latest;
ALTER TABLE loan_credit_evaluations
    DROP COLUMN asset_support_limit_minor,
    DROP COLUMN guarantee_limit_minor;
ALTER TABLE loan_credit_policy
    DROP COLUMN asset_credit_ltv_bps;
