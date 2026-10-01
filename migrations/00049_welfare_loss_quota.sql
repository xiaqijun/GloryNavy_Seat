-- Module: welfare. Period quotas use approval audit timestamps and active case state.
-- +goose Up
CREATE INDEX welfare_audit_approved_case_time ON welfare_audit(case_id,created_at) WHERE action='approve';
CREATE INDEX welfare_cases_loss_quota ON welfare_cases(account_id,corporation_id,kind) WHERE state IN ('approved','executing','cancel_requested','completed');
-- +goose Down
DROP INDEX welfare_cases_loss_quota;
DROP INDEX welfare_audit_approved_case_time;
