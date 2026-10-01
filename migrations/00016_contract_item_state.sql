-- Module: eve. Immutable items survive credential rotation and source changes.
-- +goose Up
UPDATE eve_contract_details
SET state='ready',reason='',lease_until=NULL,fence=fence+1
WHERE part='items' AND last_success_at IS NOT NULL
AND state IN ('pending','failed') AND (lease_until IS NULL OR lease_until<=now());
-- +goose Down
-- This repairs derived state; restoring the broken state would hide valid data.
SELECT 1;
