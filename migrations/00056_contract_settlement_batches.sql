-- Module: welfare. Administrator-managed batches for contract delivery checks.
-- A batch is orchestration metadata only. Each item is still settled by the
-- owning module's single-record contract verifier.
-- +goose Up
CREATE TABLE welfare_contract_settlement_batches (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    request_key uuid NOT NULL UNIQUE,
    actor_id uuid NOT NULL REFERENCES identity_users(id),
    note text NOT NULL DEFAULT '' CHECK (char_length(note) <= 500 AND note !~ '[\r\n]'),
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','processing','completed','partial','failed')),
    total_count integer NOT NULL CHECK (total_count BETWEEN 1 AND 100),
    completed_count integer NOT NULL DEFAULT 0 CHECK (completed_count >= 0),
    failed_count integer NOT NULL DEFAULT 0 CHECK (failed_count >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    finished_at timestamptz,
    next_run_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0)
);
CREATE INDEX welfare_contract_settlement_batches_due
    ON welfare_contract_settlement_batches(next_run_at, id)
    WHERE state IN ('pending','processing','partial');

CREATE TABLE welfare_contract_settlement_items (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    batch_id bigint NOT NULL REFERENCES welfare_contract_settlement_batches(id) ON DELETE CASCADE,
    source text NOT NULL CHECK (source IN ('welfare','exchange')),
    source_id bigint NOT NULL CHECK (source_id > 0),
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','processing','completed','failed')),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error text NOT NULL DEFAULT '' CHECK (char_length(last_error) <= 500 AND last_error !~ '[\r\n]'),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(batch_id, source, source_id)
);
CREATE INDEX welfare_contract_settlement_items_batch_state
    ON welfare_contract_settlement_items(batch_id, state, id);
CREATE UNIQUE INDEX welfare_contract_settlement_items_active_source
    ON welfare_contract_settlement_items(source, source_id)
    WHERE state IN ('pending','processing') OR (state='failed' AND attempts<3);

-- +goose Down
DROP TABLE welfare_contract_settlement_items;
DROP TABLE welfare_contract_settlement_batches;
