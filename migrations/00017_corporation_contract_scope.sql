-- Module: eve. Keep upstream metadata while limiting business reads and jobs.
-- +goose Up
ALTER TABLE eve_contracts ADD COLUMN in_scope boolean GENERATED ALWAYS AS (
 owner_kind <> 'corporation' OR coalesce(
  (payload->>'for_corporation'='true' AND payload->>'issuer_corporation_id'=owner_id::text)
  OR payload->>'assignee_id'=owner_id::text
  OR payload->>'acceptor_id'=owner_id::text,
 false)
) STORED NOT NULL;
CREATE INDEX eve_contracts_business_scope ON eve_contracts(owner_kind,owner_id,contract_id DESC) WHERE in_scope;
UPDATE eve_contract_details d SET state='blocked',reason='out_of_scope',lease_until=NULL,fence=fence+1
FROM eve_contracts c WHERE c.owner_kind=d.owner_kind AND c.owner_id=d.owner_id AND c.contract_id=d.contract_id AND NOT c.in_scope;
-- +goose Down
UPDATE eve_contract_details SET state=CASE WHEN part='items' AND last_success_at IS NOT NULL THEN 'ready' ELSE 'pending' END,
 reason='',next_due_at=now() WHERE reason='out_of_scope';
DROP INDEX eve_contracts_business_scope;
ALTER TABLE eve_contracts DROP COLUMN in_scope;
