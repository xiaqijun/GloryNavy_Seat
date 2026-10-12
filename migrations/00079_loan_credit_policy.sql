-- Module: loan. Configurable income-based credit policy and immutable settings history.
-- +goose Up
CREATE TABLE loan_credit_policy (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    income_window_days integer NOT NULL DEFAULT 180 CHECK (income_window_days BETWEEN 30 AND 730),
    debt_ratio_bps integer NOT NULL DEFAULT 3000 CHECK (debt_ratio_bps BETWEEN 0 AND 10000),
    term_months integer NOT NULL DEFAULT 6 CHECK (term_months BETWEEN 1 AND 60),
    score_factor_bps integer NOT NULL DEFAULT 10000 CHECK (score_factor_bps BETWEEN 0 AND 20000),
    hard_cap_minor bigint NOT NULL DEFAULT 1000000000 CHECK (hard_cap_minor >= 0),
    guarantee_cap_bps integer NOT NULL DEFAULT 10000 CHECK (guarantee_cap_bps BETWEEN 0 AND 10000),
    collateral_cap_bps integer NOT NULL DEFAULT 10000 CHECK (collateral_cap_bps BETWEEN 0 AND 10000),
    stale_after_hours integer NOT NULL DEFAULT 72 CHECK (stale_after_hours BETWEEN 1 AND 8760),
    min_income_samples integer NOT NULL DEFAULT 3 CHECK (min_income_samples BETWEEN 0 AND 12),
    insufficient_income_policy text NOT NULL DEFAULT 'fallback_pool' CHECK (insufficient_income_policy IN ('fallback_pool','zero')),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_by uuid REFERENCES identity_users(id),
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO loan_credit_policy(singleton) VALUES (true);

CREATE TABLE loan_credit_policy_audit (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_id uuid NOT NULL REFERENCES identity_users(id),
    policy_version bigint NOT NULL,
    snapshot jsonb NOT NULL CHECK (jsonb_typeof(snapshot) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX loan_credit_policy_audit_version ON loan_credit_policy_audit(policy_version DESC, id DESC);

-- +goose Down
DROP TABLE loan_credit_policy_audit;
DROP TABLE loan_credit_policy;
