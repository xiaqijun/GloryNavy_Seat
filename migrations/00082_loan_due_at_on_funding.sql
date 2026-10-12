-- Module: loan. Generate the repayment schedule when disbursement is verified.
-- +goose Up
ALTER TABLE loan_cases ALTER COLUMN first_due_at DROP NOT NULL;

-- +goose Down
ALTER TABLE loan_cases ALTER COLUMN first_due_at SET NOT NULL;
