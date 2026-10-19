-- Module: welfare. Frozen daily allocations follow the parent case's owner/state.
-- Epoch days use the Asia/Shanghai calendar. Ranges avoid a row per borrowed day.
-- +goose Up
CREATE TABLE welfare_loss_quota_days (
    case_id bigint NOT NULL REFERENCES welfare_cases(id),
    ordinal integer NOT NULL CHECK (ordinal > 0),
    start_day bigint NOT NULL CHECK (start_day >= 0),
    end_day bigint NOT NULL CHECK (end_day > start_day),
    amount_minor bigint NOT NULL CHECK (amount_minor > 0 AND amount_minor <= 100000000000000),
    PRIMARY KEY (case_id, ordinal),
    CHECK ((end_day - start_day)::numeric * amount_minor <= 100000000000000)
);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM welfare_loss_quota_days) THEN
        RAISE EXCEPTION 'welfare daily quota allocations exist; retain allocation-aware code and schema';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE welfare_loss_quota_days;
