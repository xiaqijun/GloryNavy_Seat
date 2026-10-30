-- Module: welfare. Preserve the original decision and quota allocation.
-- +goose Up
CREATE TABLE welfare_approval_reversions (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 case_id bigint NOT NULL REFERENCES welfare_cases(id),
 actor_id uuid NOT NULL,
 request_key uuid NOT NULL,
 previous_version bigint NOT NULL CHECK(previous_version > 0),
 previous_state text NOT NULL CHECK(previous_state IN ('approved','executing','rejected')),
 snapshot jsonb NOT NULL,
 quota_spans jsonb NOT NULL,
 contracts jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(actor_id,request_key)
);
CREATE INDEX welfare_approval_reversions_case ON welfare_approval_reversions(case_id,id DESC);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM welfare_approval_reversions) THEN
  RAISE EXCEPTION 'Approval reversions exist; retain reversion-aware code and history';
 END IF;
 IF EXISTS (SELECT 1 FROM welfare_audit WHERE action='revert_cancel_review') THEN
  RAISE EXCEPTION 'Welfare cancellation review reversions exist; retain reversion-aware code and history';
 END IF;
 IF EXISTS (SELECT 1 FROM loan_audit WHERE action='revert_review') THEN
  RAISE EXCEPTION 'Loan review reversions exist; retain reversion-aware code and history';
 END IF;
 IF EXISTS (SELECT 1 FROM exchange_shop_audit WHERE kind='decision' AND payload->>'state'='revert_cancel_review') THEN
  RAISE EXCEPTION 'Exchange review reversions exist; retain reversion-aware code and history';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE welfare_approval_reversions;
