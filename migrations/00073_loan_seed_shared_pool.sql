-- The loan module has one fixed pool. Its custodian is the corporation with
-- the most currently known character profiles; no pool setup page is needed.
-- +goose Up
WITH custodian AS (
  SELECT corporation_id
  FROM eve_character_profiles
  WHERE corporation_id > 0
  GROUP BY corporation_id
  ORDER BY count(*) DESC, corporation_id
  LIMIT 1
), actor AS (
  SELECT id
  FROM identity_users
  ORDER BY created_at, id
  LIMIT 1
)
INSERT INTO loan_pools
  (lender_kind, corporation_id, name, state, config, is_shared, created_by)
SELECT 'corporation', custodian.corporation_id, '全站统一贷款池', 'open',
       jsonb_build_object(
         'min_principal_minor', 100,
         'max_principal_minor', 9007199254740900,
         'max_installments', 120
       ), true, actor.id
FROM custodian CROSS JOIN actor
WHERE NOT EXISTS (SELECT 1 FROM loan_pools WHERE is_shared)
ON CONFLICT DO NOTHING;

-- +goose Down
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM loan_contributions)
     OR EXISTS (SELECT 1 FROM loan_cases c JOIN loan_pools p ON p.id=c.pool_id WHERE p.is_shared)
  THEN
    RAISE EXCEPTION 'shared loan pool has financial records; rollback is forbidden';
  END IF;
END $$;
DELETE FROM loan_pools
WHERE is_shared AND name='全站统一贷款池';
