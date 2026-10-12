-- Module: loan. Annual simple interest and immutable case rate snapshots.
-- +goose Up
ALTER TABLE loan_credit_policy
    ADD COLUMN annual_rate_bps integer NOT NULL DEFAULT 1200 CHECK (annual_rate_bps BETWEEN 0 AND 10000);

ALTER TABLE loan_cases
    ADD COLUMN interest_rate_bps integer NOT NULL DEFAULT 0 CHECK (interest_rate_bps BETWEEN 0 AND 10000);

-- +goose Down
ALTER TABLE loan_cases DROP COLUMN interest_rate_bps;
ALTER TABLE loan_credit_policy DROP COLUMN annual_rate_bps;
