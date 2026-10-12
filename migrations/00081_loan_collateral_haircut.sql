-- Module: loan. Seed the shared pool's configured collateral haircut.
-- +goose Up
UPDATE loan_pools
SET config = config || jsonb_build_object('collateral_haircut_bps', 7000), updated_at = now()
WHERE is_shared AND NOT (config ? 'collateral_haircut_bps');

-- +goose Down
UPDATE loan_pools
SET config = config - 'collateral_haircut_bps', updated_at = now()
WHERE is_shared AND config->>'collateral_haircut_bps' = '7000';
