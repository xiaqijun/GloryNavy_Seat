-- +goose Up
ALTER TABLE loan_credit_policy ADD COLUMN borrower_limit_bps integer NOT NULL DEFAULT 0 CHECK (borrower_limit_bps BETWEEN 0 AND 10000);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM loan_credit_policy WHERE borrower_limit_bps<>0)
    OR EXISTS(SELECT 1 FROM loan_credit_policy_audit WHERE snapshot ? 'borrower_limit_bps' AND COALESCE((snapshot->>'borrower_limit_bps')::integer,0)<>0)
    OR EXISTS(SELECT 1 FROM loan_credit_evaluations WHERE policy_version LIKE 'capacity-v7-policy-%') THEN
  RAISE EXCEPTION 'Loan borrower limits are in use; preserve policy and audit history';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE loan_credit_policy DROP COLUMN borrower_limit_bps;
