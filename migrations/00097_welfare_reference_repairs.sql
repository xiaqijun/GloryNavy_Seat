-- Module: welfare. Explicit operations only; do not automatically renumber history.
-- +goose Up
CREATE TABLE welfare_case_reference_repairs (
 case_id bigint PRIMARY KEY REFERENCES welfare_cases(id),
 previous_reference text NOT NULL UNIQUE CHECK(previous_reference ~ '^(SRP|PVP|WF)-[0-9]{8}-[0-9A-F]{8}-[0-9A-F]{4}-4[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12}$'),
 replacement_reference text NOT NULL UNIQUE CHECK(replacement_reference ~ '^(SRP|PVP|WF)-[0-9]{8}-[0-9]{6,19}$'),
 operator_label text NOT NULL CHECK(length(operator_label) BETWEEN 1 AND 100),
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 500),
 created_at timestamptz NOT NULL DEFAULT now(),
 CHECK(split_part(previous_reference,'-',1)=split_part(replacement_reference,'-',1)),
 CHECK(split_part(previous_reference,'-',2)=split_part(replacement_reference,'-',2))
);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM welfare_case_reference_repairs) THEN
  RAISE EXCEPTION 'Welfare reference repairs exist; retain aliases and compatible recovery program';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE welfare_case_reference_repairs;
