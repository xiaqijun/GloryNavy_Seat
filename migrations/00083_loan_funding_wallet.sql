-- Module: loan. Bind the shared pool's contract cash flow to an authorized EVE wallet owner.
-- +goose Up
ALTER TABLE loan_pools
  ADD COLUMN funding_wallet_kind text,
  ADD COLUMN funding_wallet_owner_id bigint,
  ADD COLUMN funding_wallet_division integer NOT NULL DEFAULT 0;
ALTER TABLE loan_pools
  ADD CONSTRAINT loan_pool_funding_wallet_kind_check
    CHECK (funding_wallet_kind IS NULL OR funding_wallet_kind IN ('character','corporation')),
  ADD CONSTRAINT loan_pool_funding_wallet_owner_check
    CHECK (funding_wallet_owner_id IS NULL OR funding_wallet_owner_id > 0),
  ADD CONSTRAINT loan_pool_funding_wallet_division_check
    CHECK (funding_wallet_division BETWEEN 0 AND 7),
  ADD CONSTRAINT loan_pool_funding_wallet_shape_check
    CHECK ((funding_wallet_kind IS NULL AND funding_wallet_owner_id IS NULL AND funding_wallet_division = 0)
       OR (funding_wallet_kind = 'character' AND funding_wallet_owner_id IS NOT NULL AND funding_wallet_division = 0)
       OR (funding_wallet_kind = 'corporation' AND funding_wallet_owner_id IS NOT NULL AND funding_wallet_division BETWEEN 1 AND 7));

-- Existing pools continue to use their configured custodian until a manager
-- explicitly selects a wallet. This keeps old contract evidence valid.

-- +goose Down
ALTER TABLE loan_pools
  DROP CONSTRAINT loan_pool_funding_wallet_shape_check,
  DROP CONSTRAINT loan_pool_funding_wallet_division_check,
  DROP CONSTRAINT loan_pool_funding_wallet_owner_check,
  DROP CONSTRAINT loan_pool_funding_wallet_kind_check,
  DROP COLUMN funding_wallet_division,
  DROP COLUMN funding_wallet_owner_id,
  DROP COLUMN funding_wallet_kind;
