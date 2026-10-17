-- Module: welfare. Explicit legacy reference repair history, not automatic renumbering.
-- +goose Up
CREATE TABLE welfare_contract_settlement_reference_repairs (
 batch_id bigint PRIMARY KEY REFERENCES welfare_contract_settlement_batches(id),
 previous_reference text NOT NULL UNIQUE CHECK (previous_reference ~ '^BATCH-[0-9]{8}-[0-9A-F]{8}-[0-9A-F]{4}-4[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12}$'),
 replacement_reference text NOT NULL UNIQUE CHECK (replacement_reference ~ '^BATCH-[0-9]{8}-[0-9]{6,19}$'),
 operator_label text NOT NULL CHECK (length(operator_label) BETWEEN 1 AND 100),
 reason text NOT NULL CHECK (length(reason) BETWEEN 1 AND 500),
 created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM welfare_contract_settlement_reference_repairs) THEN
  RAISE EXCEPTION 'Settlement reference repairs exist; preserve aliases and use a compatible recovery version';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE welfare_contract_settlement_reference_repairs;
