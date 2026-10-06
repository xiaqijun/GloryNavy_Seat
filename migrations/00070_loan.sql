-- Module: loan. ISK lending between a configured personal/corporation pool and a member.
-- Thresholds, interest terms and collateral haircuts are configuration, never implicit defaults.
-- +goose Up
CREATE TABLE loan_pools (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    lender_kind text NOT NULL CHECK (lender_kind IN ('personal','corporation')),
    lender_user_id uuid REFERENCES identity_users(id),
    corporation_id bigint CHECK (corporation_id IS NULL OR corporation_id > 0),
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 160),
    state text NOT NULL DEFAULT 'paused' CHECK (state IN ('open','paused','closed')),
    config jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(config) = 'object'),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_by uuid NOT NULL REFERENCES identity_users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((lender_kind='personal' AND lender_user_id IS NOT NULL AND corporation_id IS NULL)
        OR (lender_kind='corporation' AND lender_user_id IS NULL AND corporation_id IS NOT NULL))
);
CREATE INDEX loan_pools_lender ON loan_pools(lender_kind, lender_user_id, corporation_id, state);

CREATE TABLE loan_credit_profiles (
    account_id uuid PRIMARY KEY REFERENCES identity_users(id) ON DELETE CASCADE,
    score integer CHECK (score IS NULL OR score BETWEEN 0 AND 100),
    total_limit_minor bigint NOT NULL DEFAULT 0 CHECK (total_limit_minor >= 0),
    unsecured_limit_minor bigint NOT NULL DEFAULT 0 CHECK (unsecured_limit_minor >= 0),
    state text NOT NULL DEFAULT 'unconfigured' CHECK (state IN ('unconfigured','active','suspended')),
    rule_version text NOT NULL DEFAULT '',
    reason text NOT NULL DEFAULT '' CHECK (length(reason) <= 500),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_by uuid REFERENCES identity_users(id),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE loan_credit_history (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id uuid NOT NULL REFERENCES identity_users(id) ON DELETE CASCADE,
    score integer CHECK (score IS NULL OR score BETWEEN 0 AND 100),
    total_limit_minor bigint NOT NULL CHECK (total_limit_minor >= 0),
    unsecured_limit_minor bigint NOT NULL CHECK (unsecured_limit_minor >= 0),
    state text NOT NULL,
    rule_version text NOT NULL,
    explanation jsonb NOT NULL DEFAULT '{}'::jsonb,
    actor_id uuid NOT NULL REFERENCES identity_users(id),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX loan_credit_history_account ON loan_credit_history(account_id, id DESC);

CREATE TABLE loan_cases (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id text NOT NULL UNIQUE,
    pool_id bigint NOT NULL REFERENCES loan_pools(id),
    borrower_account_id uuid NOT NULL REFERENCES identity_users(id),
    borrower_character_id bigint NOT NULL CHECK (borrower_character_id > 0),
    principal_minor bigint NOT NULL CHECK (principal_minor > 0),
    interest_minor bigint NOT NULL CHECK (interest_minor >= 0),
    total_due_minor bigint NOT NULL CHECK (total_due_minor = principal_minor + interest_minor),
    installment_count integer NOT NULL CHECK (installment_count BETWEEN 1 AND 120),
    interval_days integer NOT NULL CHECK (interval_days BETWEEN 1 AND 365),
    first_due_at timestamptz NOT NULL,
    state text NOT NULL DEFAULT 'submitted' CHECK (state IN ('submitted','awaiting_acceptance','approved','funding','active','settled','rejected','cancel_requested','cancelled','defaulted','disputed')),
    terms_version bigint NOT NULL DEFAULT 1 CHECK (terms_version > 0),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    reviewer_id uuid REFERENCES identity_users(id),
    review_note text NOT NULL DEFAULT '' CHECK (length(review_note) <= 1000),
    created_at timestamptz NOT NULL DEFAULT now(),
    accepted_at timestamptz,
    funded_at timestamptz,
    settled_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX loan_cases_borrower ON loan_cases(borrower_account_id, created_at DESC);
CREATE INDEX loan_cases_pool_state ON loan_cases(pool_id, state, created_at DESC);

CREATE TABLE loan_installments (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    case_id bigint NOT NULL REFERENCES loan_cases(id) ON DELETE CASCADE,
    sequence integer NOT NULL CHECK (sequence > 0),
    due_at timestamptz NOT NULL,
    principal_minor bigint NOT NULL CHECK (principal_minor >= 0),
    interest_minor bigint NOT NULL CHECK (interest_minor >= 0),
    paid_principal_minor bigint NOT NULL DEFAULT 0 CHECK (paid_principal_minor >= 0),
    paid_interest_minor bigint NOT NULL DEFAULT 0 CHECK (paid_interest_minor >= 0),
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','partial','paid','overdue')),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    UNIQUE(case_id, sequence),
    CHECK (paid_principal_minor <= principal_minor),
    CHECK (paid_interest_minor <= interest_minor)
);
CREATE INDEX loan_installments_due ON loan_installments(case_id, state, due_at, sequence);

CREATE TABLE loan_guarantees (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    case_id bigint NOT NULL REFERENCES loan_cases(id) ON DELETE CASCADE,
    guarantor_account_id uuid NOT NULL REFERENCES identity_users(id),
    amount_minor bigint NOT NULL CHECK (amount_minor > 0),
    state text NOT NULL DEFAULT 'invited' CHECK (state IN ('invited','accepted','rejected','active','released','called','disputed')),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    note text NOT NULL DEFAULT '' CHECK (length(note) <= 500),
    created_at timestamptz NOT NULL DEFAULT now(),
    decided_at timestamptz,
    UNIQUE(case_id, guarantor_account_id)
);

CREATE TABLE loan_collateral (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    case_id bigint NOT NULL REFERENCES loan_cases(id) ON DELETE CASCADE,
    owner_account_id uuid NOT NULL REFERENCES identity_users(id),
    contract_kind text NOT NULL CHECK (contract_kind IN ('character','corporation')),
    contract_owner_id bigint NOT NULL CHECK (contract_owner_id > 0),
    contract_id bigint NOT NULL CHECK (contract_id > 0),
    items jsonb NOT NULL CHECK (jsonb_typeof(items) = 'array'),
    valuation_minor bigint NOT NULL CHECK (valuation_minor > 0),
    haircut_bps integer NOT NULL CHECK (haircut_bps BETWEEN 1 AND 10000),
    covered_minor bigint NOT NULL CHECK (covered_minor > 0),
    state text NOT NULL DEFAULT 'proposed' CHECK (state IN ('proposed','approved','held','released','disputed')),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    decided_at timestamptz
);
CREATE UNIQUE INDEX loan_collateral_contract ON loan_collateral(contract_kind, contract_owner_id, contract_id);

CREATE TABLE loan_payment_intents (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id text NOT NULL UNIQUE,
    case_id bigint NOT NULL REFERENCES loan_cases(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('disbursement','repayment')),
    payer_account_id uuid REFERENCES identity_users(id),
    recipient_account_id uuid REFERENCES identity_users(id),
    recipient_character_id bigint NOT NULL CHECK (recipient_character_id > 0),
    expected_minor bigint NOT NULL CHECK (expected_minor > 0),
    contract_kind text NOT NULL CHECK (contract_kind IN ('character','corporation')),
    contract_owner_id bigint NOT NULL CHECK (contract_owner_id > 0),
    contract_id bigint NOT NULL CHECK (contract_id > 0),
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','verified','rejected','partial','disputed')),
    evidence jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(evidence) = 'object'),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_by uuid NOT NULL REFERENCES identity_users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    verified_at timestamptz,
    UNIQUE(contract_kind, contract_owner_id, contract_id)
);
CREATE INDEX loan_payment_case ON loan_payment_intents(case_id, kind, created_at DESC);

CREATE TABLE loan_audit (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    case_id bigint REFERENCES loan_cases(id) ON DELETE SET NULL,
    actor_id uuid REFERENCES identity_users(id),
    action text NOT NULL,
    before_state jsonb NOT NULL DEFAULT '{}'::jsonb,
    after_state jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX loan_audit_case ON loan_audit(case_id, id DESC);

-- +goose Down
DROP TABLE loan_audit;
DROP TABLE loan_payment_intents;
DROP TABLE loan_collateral;
DROP TABLE loan_guarantees;
DROP TABLE loan_installments;
DROP TABLE loan_cases;
DROP TABLE loan_credit_history;
DROP TABLE loan_credit_profiles;
DROP TABLE loan_pools;
