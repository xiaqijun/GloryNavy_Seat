-- Module: loan. Freeze the automatically allocated excess and selection value.
-- Zero-valued rows are legacy requests and retain their original delivery rules.
-- +goose Up
ALTER TABLE loan_collateral_requests
    ADD COLUMN coverage_minor bigint NOT NULL DEFAULT 0,
    ADD COLUMN valuation_minor bigint NOT NULL DEFAULT 0,
    ADD CONSTRAINT loan_collateral_request_value CHECK (
        (coverage_minor = 0 AND valuation_minor = 0) OR
        (coverage_minor > 0 AND valuation_minor > 0 AND
         coverage_minor <= floor(valuation_minor::numeric * 0.8))
    );

-- Retain rejected/released invitation history while allowing a new amount.
ALTER TABLE loan_guarantees DROP CONSTRAINT loan_guarantees_case_id_guarantor_account_id_key;
CREATE UNIQUE INDEX loan_guarantees_current_account ON loan_guarantees(case_id,guarantor_account_id)
    WHERE state NOT IN ('rejected','released');

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM loan_collateral_requests WHERE coverage_minor > 0) THEN
        RAISE EXCEPTION 'Cannot discard frozen collateral coverage targets';
    END IF;
    IF EXISTS (SELECT 1 FROM loan_guarantees GROUP BY case_id,guarantor_account_id HAVING count(*) > 1) THEN
        RAISE EXCEPTION 'Cannot discard repeated guarantee invitation history';
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE loan_collateral_requests
    DROP CONSTRAINT loan_collateral_request_value,
    DROP COLUMN coverage_minor,
    DROP COLUMN valuation_minor;

DROP INDEX loan_guarantees_current_account;
ALTER TABLE loan_guarantees ADD CONSTRAINT loan_guarantees_case_id_guarantor_account_id_key UNIQUE(case_id,guarantor_account_id);
