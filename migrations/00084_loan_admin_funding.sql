-- Module: loan. Administrators may record capital added directly to the shared pool.
-- The entry is an auditable ledger deposit and does not require an EVE contract.
-- +goose Up
DO $$
DECLARE
  constraint_name text;
BEGIN
  FOR constraint_name IN
    SELECT conname
    FROM pg_constraint
    WHERE conrelid = 'loan_pool_ledger'::regclass
      AND contype = 'c'
  LOOP
    EXECUTE format('ALTER TABLE loan_pool_ledger DROP CONSTRAINT %I', constraint_name);
  END LOOP;
END $$;

ALTER TABLE loan_pool_ledger
  ADD CONSTRAINT loan_pool_ledger_delta_check CHECK (delta_minor <> 0),
  ADD CONSTRAINT loan_pool_ledger_flow_check CHECK (
    (kind='deposit' AND contribution_id IS NOT NULL AND payment_id IS NULL AND delta_minor>0)
    OR (kind='admin_deposit' AND contribution_id IS NULL AND payment_id IS NULL AND delta_minor>0)
    OR (kind='disbursement' AND contribution_id IS NULL AND payment_id IS NOT NULL AND delta_minor<0)
    OR (kind='repayment' AND contribution_id IS NULL AND payment_id IS NOT NULL AND delta_minor>0)
  );

-- +goose Down
ALTER TABLE loan_pool_ledger
  DROP CONSTRAINT loan_pool_ledger_flow_check,
  DROP CONSTRAINT loan_pool_ledger_delta_check;
ALTER TABLE loan_pool_ledger
  ADD CONSTRAINT loan_pool_ledger_delta_check CHECK (delta_minor <> 0),
  ADD CONSTRAINT loan_pool_ledger_flow_check CHECK (
    (kind='deposit' AND contribution_id IS NOT NULL AND payment_id IS NULL AND delta_minor>0)
    OR (kind='disbursement' AND contribution_id IS NULL AND payment_id IS NOT NULL AND delta_minor<0)
    OR (kind='repayment' AND contribution_id IS NULL AND payment_id IS NOT NULL AND delta_minor>0)
  );
