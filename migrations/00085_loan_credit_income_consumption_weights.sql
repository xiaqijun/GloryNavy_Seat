-- Module: loan. Use separate income and consumption evidence for credit capacity.
-- +goose Up
ALTER TABLE loan_credit_policy
  ADD COLUMN income_weight_bps integer NOT NULL DEFAULT 7000
    CHECK (income_weight_bps BETWEEN 0 AND 10000);

-- +goose Down
ALTER TABLE loan_credit_policy DROP COLUMN income_weight_bps;
