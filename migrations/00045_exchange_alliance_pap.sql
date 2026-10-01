-- Module: exchange. Register the independently configured alliance PAP source.
-- +goose Up
INSERT INTO exchange_source_rates(source_id) VALUES ('alliance_pap') ON CONFLICT (source_id) DO NOTHING;
-- +goose Down
DELETE FROM exchange_source_rates WHERE source_id='alliance_pap';
