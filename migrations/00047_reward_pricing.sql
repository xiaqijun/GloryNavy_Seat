-- Module: exchange. Mixed physical/ISK rewards and persistent six-hour pricing.
-- +goose Up
ALTER TABLE attendance_alliance_pap_snapshot ADD COLUMN original_account_id uuid;
ALTER TABLE exchange_rewards DROP CONSTRAINT exchange_rewards_type_id_check;
ALTER TABLE exchange_rewards ADD CONSTRAINT exchange_rewards_type_id_check CHECK(type_id>=0);
CREATE TABLE exchange_reward_pricing (
 reward_id bigint PRIMARY KEY REFERENCES exchange_rewards(id),
 automatic boolean NOT NULL DEFAULT true,
 revision bigint NOT NULL DEFAULT 1,
 next_at timestamptz NOT NULL DEFAULT now(),
 checked_at timestamptz,
 quoted_at timestamptz,
 status text NOT NULL DEFAULT 'pending' CHECK(status IN('pending','ready','incomplete','failed','manual'))
);
-- Preserve existing manually chosen values on upgrade.
INSERT INTO exchange_reward_pricing(reward_id,automatic,status) SELECT id,false,'manual' FROM exchange_rewards;
CREATE INDEX exchange_reward_pricing_due ON exchange_reward_pricing(next_at,reward_id) WHERE automatic;
-- +goose Down
-- Refuse to discard the meaning of persisted cash rewards (including order snapshots).
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM attendance_alliance_pap_snapshot WHERE original_account_id IS NOT NULL)
 THEN RAISE EXCEPTION 'merged alliance PAP ownership history requires schema 47'; END IF;
 IF EXISTS(SELECT 1 FROM exchange_rewards WHERE type_id=0 OR coalesce((content->>'isk_minor')::bigint,0)>0)
 OR EXISTS(SELECT 1 FROM exchange_redemptions WHERE coalesce((reward_content->>'isk_minor')::bigint,0)>0)
 THEN RAISE EXCEPTION 'cash reward snapshots require the mixed reward implementation'; END IF;
END $$;
-- +goose StatementEnd
DROP TABLE exchange_reward_pricing;
ALTER TABLE attendance_alliance_pap_snapshot DROP COLUMN original_account_id;
ALTER TABLE exchange_rewards DROP CONSTRAINT exchange_rewards_type_id_check;
ALTER TABLE exchange_rewards ADD CONSTRAINT exchange_rewards_type_id_check CHECK(type_id>0);
