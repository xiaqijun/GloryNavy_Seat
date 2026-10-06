-- Module: loan. One custodial pool; lending commitments are not cash.
-- +goose Up
ALTER TABLE loan_pools ADD COLUMN is_shared boolean NOT NULL DEFAULT false;
ALTER TABLE loan_pools ADD COLUMN custodian_character_id bigint CHECK (custodian_character_id > 0);
CREATE UNIQUE INDEX loan_single_shared_pool ON loan_pools(is_shared) WHERE is_shared;

CREATE TABLE loan_contributions (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    pool_id bigint NOT NULL REFERENCES loan_pools(id),
    account_id uuid NOT NULL REFERENCES identity_users(id),
    original_account_id uuid REFERENCES identity_users(id),
    lender_kind text NOT NULL CHECK (lender_kind IN ('personal','corporation')),
    source_character_id bigint NOT NULL CHECK (source_character_id > 0),
    corporation_id bigint CHECK (corporation_id > 0),
    amount_minor bigint NOT NULL CHECK (amount_minor > 0 AND amount_minor % 100 = 0),
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','funded','cancelled')),
    contract_kind text CHECK (contract_kind IN ('character','corporation')),
    contract_owner_id bigint CHECK (contract_owner_id > 0),
    contract_id bigint UNIQUE CHECK (contract_id > 0),
    evidence jsonb NOT NULL DEFAULT '{}'::jsonb,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    funded_at timestamptz,
    CHECK ((lender_kind='personal' AND corporation_id IS NULL) OR (lender_kind='corporation' AND corporation_id IS NOT NULL))
);
CREATE INDEX loan_contributions_account ON loan_contributions(account_id,id DESC);
CREATE TABLE loan_pool_ledger (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    pool_id bigint NOT NULL REFERENCES loan_pools(id),
    contribution_id bigint UNIQUE REFERENCES loan_contributions(id),
    payment_id bigint UNIQUE REFERENCES loan_payment_intents(id),
    delta_minor bigint NOT NULL CHECK (delta_minor <> 0),
    kind text NOT NULL CHECK (kind IN ('deposit','disbursement','repayment')),
    actor_id uuid NOT NULL REFERENCES identity_users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((kind='deposit' AND contribution_id IS NOT NULL AND payment_id IS NULL AND delta_minor>0)
       OR (kind='disbursement' AND contribution_id IS NULL AND payment_id IS NOT NULL AND delta_minor<0)
       OR (kind='repayment' AND contribution_id IS NULL AND payment_id IS NOT NULL AND delta_minor>0))
);
CREATE INDEX loan_pool_ledger_pool ON loan_pool_ledger(pool_id,id);
ALTER TABLE loan_cases ADD COLUMN original_account_id uuid REFERENCES identity_users(id);
ALTER TABLE loan_pools ADD COLUMN original_account_id uuid REFERENCES identity_users(id);
ALTER TABLE loan_guarantees ADD COLUMN original_account_id uuid REFERENCES identity_users(id);
ALTER TABLE loan_collateral ADD COLUMN original_account_id uuid REFERENCES identity_users(id);

-- +goose Down
-- Refuse rollback once funds or shared loans exist; never silently lose cash evidence.
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM loan_contributions) OR EXISTS(SELECT 1 FROM loan_cases c JOIN loan_pools p ON p.id=c.pool_id WHERE p.is_shared) THEN
   RAISE EXCEPTION 'shared loan pool has financial records; rollback is forbidden';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE loan_pool_ledger;
DROP TABLE loan_contributions;
ALTER TABLE loan_cases DROP COLUMN original_account_id;
ALTER TABLE loan_guarantees DROP COLUMN original_account_id;
ALTER TABLE loan_collateral DROP COLUMN original_account_id;
ALTER TABLE loan_pools DROP COLUMN original_account_id;
DROP INDEX loan_single_shared_pool;
ALTER TABLE loan_pools DROP COLUMN is_shared, DROP COLUMN custodian_character_id;
