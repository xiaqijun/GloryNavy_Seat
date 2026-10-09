-- Module: approval. Central list projection; source modules remain the owners
-- of approval state, evidence, contracts and financial records.
-- +goose Up
CREATE TABLE approval_items (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source text NOT NULL CHECK (length(source) BETWEEN 1 AND 80),
    source_id bigint NOT NULL CHECK (source_id > 0),
    source_version bigint NOT NULL DEFAULT 0 CHECK (source_version >= 0),
    account_id uuid NOT NULL,
    corporation_id bigint,
    kind text NOT NULL,
    bucket text NOT NULL CHECK (bucket IN ('pending','information','fulfillment','exceptions','history')),
    state text NOT NULL,
    status text NOT NULL,
    applicant text NOT NULL DEFAULT '',
    recipient text NOT NULL DEFAULT '',
    title text NOT NULL DEFAULT '',
    reference text NOT NULL DEFAULT '',
    amount_minor bigint NOT NULL DEFAULT 0,
    unit text NOT NULL DEFAULT '',
    occurred_at timestamptz NOT NULL,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    actions jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(actions) = 'array'),
    projection_state text NOT NULL DEFAULT 'fresh' CHECK (projection_state IN ('fresh','stale','error')),
    projection_error text NOT NULL DEFAULT '',
    projected_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(source, source_id)
);
CREATE INDEX approval_items_bucket_time ON approval_items(bucket, occurred_at, id);
CREATE INDEX approval_items_bucket_id ON approval_items(bucket, id);
CREATE INDEX approval_items_corporation_time ON approval_items(corporation_id, bucket, occurred_at, id);
CREATE INDEX approval_items_account_time ON approval_items(account_id, bucket, occurred_at, id);
CREATE INDEX approval_items_source_state ON approval_items(source, state, occurred_at, id);

CREATE TABLE approval_projection_runs (
    source text PRIMARY KEY,
    state text NOT NULL CHECK (state IN ('running','fresh','stale','error')),
    last_started_at timestamptz,
    last_completed_at timestamptz,
    last_error text NOT NULL DEFAULT '',
    item_count bigint NOT NULL DEFAULT 0 CHECK (item_count >= 0)
);

-- +goose Down
DROP TABLE approval_projection_runs;
DROP TABLE approval_items;
