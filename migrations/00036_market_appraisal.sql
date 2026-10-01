-- Module: market. Global appraisal policy; no account balances or payouts.
-- +goose Up
CREATE TABLE market_settings(id boolean PRIMARY KEY DEFAULT true CHECK(id), ratio_bps integer NOT NULL CHECK(ratio_bps BETWEEN 0 AND 100000), version bigint NOT NULL DEFAULT 1, updated_at timestamptz NOT NULL DEFAULT now());
INSERT INTO market_settings(id,ratio_bps) VALUES(true,10000);
CREATE TABLE market_settings_audit(id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, actor_id uuid NOT NULL, ratio_bps integer NOT NULL, version bigint NOT NULL, created_at timestamptz NOT NULL DEFAULT now());
-- +goose Down
DROP TABLE market_settings_audit;
DROP TABLE market_settings;
