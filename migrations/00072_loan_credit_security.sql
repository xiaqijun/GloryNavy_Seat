-- Module: loan. Versioned credit evidence and character-addressed guarantees.
-- +goose Up
ALTER TABLE loan_guarantees
    ADD COLUMN guarantor_character_id bigint CHECK (guarantor_character_id > 0);
CREATE INDEX loan_guarantees_guarantor ON loan_guarantees(guarantor_account_id, state, id DESC);

CREATE TABLE loan_credit_evaluations (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id uuid NOT NULL REFERENCES identity_users(id) ON DELETE CASCADE,
    original_account_id uuid REFERENCES identity_users(id),
    policy_version text NOT NULL CHECK (length(policy_version) BETWEEN 1 AND 80),
    score integer NOT NULL CHECK (score BETWEEN 0 AND 100),
    total_limit_minor bigint NOT NULL CHECK (total_limit_minor >= 0),
    unsecured_limit_minor bigint NOT NULL CHECK (unsecured_limit_minor >= 0),
    state text NOT NULL CHECK (state IN ('active','suspended','provisional')),
    factor_scores jsonb NOT NULL CHECK (jsonb_typeof(factor_scores) = 'object'),
    evidence jsonb NOT NULL CHECK (jsonb_typeof(evidence) = 'object'),
    evidence_cutoff timestamptz NOT NULL,
    evaluated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(account_id, id)
);
CREATE INDEX loan_credit_evaluations_account ON loan_credit_evaluations(account_id, id DESC);

-- +goose Down
DROP TABLE loan_credit_evaluations;
DROP INDEX loan_guarantees_guarantor;
ALTER TABLE loan_guarantees DROP COLUMN guarantor_character_id;
