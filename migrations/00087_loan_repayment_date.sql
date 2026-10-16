-- Module: loan. An optional fixed maturity for single-payment loans.
-- +goose Up
ALTER TABLE loan_cases ADD COLUMN requested_due_at timestamptz;
ALTER TABLE loan_cases ADD CONSTRAINT loan_fixed_due_single_payment
    CHECK (requested_due_at IS NULL OR installment_count = 1);

-- +goose Down
-- Do not silently discard an agreed deadline or restore legacy interest rules.
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM loan_cases WHERE requested_due_at IS NOT NULL) THEN
        RAISE EXCEPTION 'Cannot remove fixed repayment dates while dated loans exist';
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE loan_cases DROP CONSTRAINT loan_fixed_due_single_payment;
ALTER TABLE loan_cases DROP COLUMN requested_due_at;
