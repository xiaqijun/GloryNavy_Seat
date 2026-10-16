-- Module: loan. Selected personal assets are a delivery request, not collateral coverage.
-- +goose Up
CREATE TABLE loan_collateral_requests (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id text NOT NULL UNIQUE,
    case_id bigint NOT NULL REFERENCES loan_cases(id),
    source_character_id bigint NOT NULL CHECK (source_character_id > 0),
    recipient_character_id bigint NOT NULL CHECK (recipient_character_id > 0),
    selections jsonb NOT NULL CHECK (jsonb_typeof(selections) = 'array'),
    items jsonb NOT NULL CHECK (jsonb_typeof(items) = 'array'),
    observed_at timestamptz NOT NULL,
    state text NOT NULL DEFAULT 'awaiting_delivery' CHECK (state IN ('awaiting_delivery','delivered','cancelled')),
    collateral_id bigint UNIQUE REFERENCES loan_collateral(id),
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((state = 'delivered') = (collateral_id IS NOT NULL))
);
CREATE INDEX loan_collateral_requests_case ON loan_collateral_requests(case_id,id);
CREATE INDEX loan_collateral_requests_pending ON loan_collateral_requests(source_character_id) WHERE state = 'awaiting_delivery';

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM loan_collateral_requests) THEN
        RAISE EXCEPTION 'Cannot discard saved collateral delivery requests';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE loan_collateral_requests;
