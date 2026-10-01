-- +goose Up
ALTER TABLE welfare_cases DROP CONSTRAINT welfare_cases_state_check;
ALTER TABLE welfare_cases ADD CONSTRAINT welfare_cases_state_check CHECK (state IN ('submitted','information','external','approved','executing','completed','rejected','cancelled','reversed','cancel_requested'));

-- +goose Down
-- Pending reviews must be resolved before returning to a program that cannot see them.
ALTER TABLE welfare_cases DROP CONSTRAINT welfare_cases_state_check;
ALTER TABLE welfare_cases ADD CONSTRAINT welfare_cases_state_check CHECK (state IN ('submitted','information','external','approved','executing','completed','rejected','cancelled','reversed'));
