-- Module: welfare. Policies remain disabled until explicitly configured. No awards are issued.
-- +goose Up
CREATE TABLE welfare_policies (
 corporation_id bigint NOT NULL, kind text NOT NULL, version bigint NOT NULL DEFAULT 1,
 config jsonb NOT NULL, PRIMARY KEY(corporation_id,kind)
);
CREATE TABLE welfare_members (
 account_id uuid PRIMARY KEY, original_account_id uuid, verified boolean NOT NULL DEFAULT false,
 history jsonb NOT NULL DEFAULT '{}', months jsonb NOT NULL DEFAULT '[]', version bigint NOT NULL DEFAULT 1
);
CREATE TABLE welfare_cases (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, account_id uuid NOT NULL, original_account_id uuid,
 corporation_id bigint NOT NULL, kind text NOT NULL,
 state text NOT NULL CHECK(state IN ('submitted','information','external','approved','executing','completed','rejected','cancelled','reversed')),
 version bigint NOT NULL DEFAULT 1, detail jsonb NOT NULL, award_minor bigint NOT NULL DEFAULT 0 CHECK(award_minor>=0),
 claim_keys text[] NOT NULL DEFAULT '{}', created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX welfare_cases_owner ON welfare_cases(account_id,id DESC);
CREATE INDEX welfare_cases_corporation ON welfare_cases(corporation_id,id DESC);
CREATE TABLE welfare_claims (claim_key text PRIMARY KEY, case_id bigint NOT NULL REFERENCES welfare_cases(id));
CREATE TABLE welfare_audit (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, actor_id uuid NOT NULL, request_key uuid NOT NULL,
 fingerprint text NOT NULL, case_id bigint REFERENCES welfare_cases(id), action text NOT NULL, note text NOT NULL,
 result jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(actor_id,request_key)
);
CREATE TRIGGER welfare_members_active_account BEFORE INSERT OR UPDATE ON welfare_members FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('account_id');
CREATE TRIGGER welfare_cases_active_account BEFORE INSERT OR UPDATE ON welfare_cases FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('account_id');
CREATE TRIGGER welfare_audit_active_actor BEFORE INSERT ON welfare_audit FOR EACH ROW EXECUTE FUNCTION identity_guard_active_account('actor_id');
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM welfare_cases) THEN RAISE EXCEPTION 'Welfare records exist; preserve claims, delivery and currency provenance'; END IF;
END $$;
-- +goose StatementEnd
DROP TABLE welfare_audit;
DROP TABLE welfare_claims;
DROP TABLE welfare_cases;
DROP TABLE welfare_members;
DROP TABLE welfare_policies;
