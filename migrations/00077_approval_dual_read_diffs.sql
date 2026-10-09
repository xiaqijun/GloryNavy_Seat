-- Module: approval. Persist P2 dual-read diagnostics without copying source state.
-- +goose Up
CREATE TABLE approval_dual_read_diffs (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_id text NOT NULL CHECK (length(actor_id) BETWEEN 1 AND 200),
    filter jsonb NOT NULL DEFAULT '{}'::jsonb,
    reason text NOT NULL CHECK (length(reason) BETWEEN 1 AND 200),
    source text NOT NULL DEFAULT '',
    source_id bigint,
    indexed_version bigint,
    legacy_version bigint,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX approval_dual_read_diffs_created ON approval_dual_read_diffs(created_at DESC, id DESC);
CREATE INDEX approval_dual_read_diffs_reason ON approval_dual_read_diffs(reason, created_at DESC);
CREATE INDEX approval_dual_read_diffs_source ON approval_dual_read_diffs(source, created_at DESC) WHERE source <> '';

-- +goose Down
DROP TABLE approval_dual_read_diffs;
