-- name: GetContractCursor :one
SELECT * FROM eve_contract_cursors WHERE target_id=$1;
-- name: SaveContractCursor :exec
INSERT INTO eve_contract_cursors(target_id,generation,owner_id,page,pages,expires_at) VALUES($1,$2,$3,$4,$5,$6)
ON CONFLICT(target_id) DO UPDATE SET generation=excluded.generation,owner_id=excluded.owner_id,page=excluded.page,pages=excluded.pages,expires_at=excluded.expires_at;
-- name: DeleteContractCursor :exec
DELETE FROM eve_contract_cursors WHERE target_id=$1;
-- name: ContractCorporationSource :one
SELECT c.character_id FROM eve_credentials c JOIN eve_role_snapshots s USING(character_id)
JOIN eve_sync_targets t ON t.character_id=c.character_id AND t.resource='corporation_contracts'
WHERE s.corporation_id=$1 AND s.valid_until>now() AND s.owner_hash=c.owner_hash
AND c.state IN ('ready','retry') AND (c.roles_not_before IS NULL OR c.roles_not_before<=now())
AND 'esi-contracts.read_corporation_contracts.v1'=ANY(c.scopes) AND t.state<>'blocked'
ORDER BY c.character_id LIMIT 1;
-- name: UpsertContract :one
INSERT INTO eve_contracts(owner_kind,owner_id,contract_id,source_character_id,source_generation,contract_type,status,payload)
VALUES($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT(owner_kind,owner_id,contract_id) DO UPDATE SET source_character_id=excluded.source_character_id,
 source_generation=excluded.source_generation,contract_type=excluded.contract_type,status=excluded.status,payload=excluded.payload,checked_at=now()
RETURNING in_scope;
-- name: GetContract :one
SELECT * FROM eve_contracts WHERE owner_kind=$1 AND owner_id=$2 AND contract_id=$3 AND in_scope;
-- name: ExcludeContractDetails :exec
UPDATE eve_contract_details SET state='blocked',reason='out_of_scope',lease_until=NULL,fence=fence+1
WHERE owner_kind=$1 AND owner_id=$2 AND contract_id=$3 AND reason<>'out_of_scope';
-- name: PrepareContractDetail :one
INSERT INTO eve_contract_details(owner_kind,owner_id,contract_id,part,target_id,source_character_id,generation)
VALUES($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT(owner_kind,owner_id,contract_id,part) DO UPDATE SET
 target_id=excluded.target_id,source_character_id=excluded.source_character_id,generation=excluded.generation,
 state=CASE WHEN eve_contract_details.part='items' AND eve_contract_details.last_success_at IS NOT NULL THEN 'ready' WHEN eve_contract_details.reason='out_of_scope' OR eve_contract_details.source_character_id<>excluded.source_character_id OR eve_contract_details.generation<>excluded.generation THEN 'pending' ELSE eve_contract_details.state END,
 next_due_at=CASE WHEN eve_contract_details.reason='out_of_scope' THEN now() ELSE eve_contract_details.next_due_at END,
 page=CASE WHEN eve_contract_details.source_character_id<>excluded.source_character_id OR eve_contract_details.generation<>excluded.generation THEN 1 ELSE eve_contract_details.page END,
 pages=CASE WHEN eve_contract_details.source_character_id<>excluded.source_character_id OR eve_contract_details.generation<>excluded.generation THEN 0 ELSE eve_contract_details.pages END,
 reason=CASE WHEN eve_contract_details.reason='out_of_scope' OR (eve_contract_details.part='items' AND eve_contract_details.last_success_at IS NOT NULL) OR eve_contract_details.source_character_id<>excluded.source_character_id OR eve_contract_details.generation<>excluded.generation THEN '' ELSE eve_contract_details.reason END,
 lease_until=CASE WHEN eve_contract_details.source_character_id<>excluded.source_character_id OR eve_contract_details.generation<>excluded.generation THEN NULL ELSE eve_contract_details.lease_until END
RETURNING *;
-- name: ClaimContractDetail :one
UPDATE eve_contract_details SET lease_until=now()+interval '90 seconds',fence=fence+1,state='running'
WHERE owner_kind=$1 AND owner_id=$2 AND contract_id=$3 AND part=$4 AND generation=$5 AND source_character_id=$6
AND (lease_until IS NULL OR lease_until<now()) RETURNING *;
-- name: LockContractDetail :one
SELECT * FROM eve_contract_details WHERE owner_kind=$1 AND owner_id=$2 AND contract_id=$3 AND part=$4 FOR UPDATE;
-- name: GetContractDetail :one
SELECT * FROM eve_contract_details WHERE owner_kind=$1 AND owner_id=$2 AND contract_id=$3 AND part=$4;
-- name: QueueContractDetail :exec
UPDATE eve_contract_details SET state='pending',reason='' WHERE owner_kind=$1 AND owner_id=$2 AND contract_id=$3 AND part=$4 AND state<>'running';
-- name: FinishContractDetail :exec
UPDATE eve_contract_details SET state=$5,reason=$6,next_due_at=$7,page=$8,pages=$9,lease_until=NULL,
last_success_at=CASE WHEN $5='ready' THEN now() ELSE last_success_at END
WHERE owner_kind=$1 AND owner_id=$2 AND contract_id=$3 AND part=$4;
-- name: ReplaceContractItems :exec
DELETE FROM eve_contract_items WHERE owner_kind=$1 AND owner_id=$2 AND contract_id=$3;
-- name: InsertContractItems :exec
INSERT INTO eve_contract_items(owner_kind,owner_id,contract_id,record_id,type_id,quantity,is_included,is_singleton,raw_quantity)
SELECT sqlc.arg(owner_kind),sqlc.arg(owner_id),sqlc.arg(contract_id),x.record_id,x.type_id,x.quantity,x.is_included,x.is_singleton,x.raw_quantity FROM jsonb_to_recordset(sqlc.arg(items)::jsonb)
AS x(record_id bigint,type_id bigint,quantity bigint,is_included boolean,is_singleton boolean,raw_quantity bigint);
-- name: InsertContractBids :exec
INSERT INTO eve_contract_bids(owner_kind,owner_id,contract_id,bid_id,bidder_id,amount,date_bid)
SELECT sqlc.arg(owner_kind),sqlc.arg(owner_id),sqlc.arg(contract_id),x.bid_id,x.bidder_id,x.amount,x.date_bid FROM jsonb_to_recordset(sqlc.arg(bids)::jsonb)
AS x(bid_id bigint,bidder_id bigint,amount numeric,date_bid timestamptz)
ON CONFLICT(owner_kind,owner_id,contract_id,bid_id) DO UPDATE SET amount=excluded.amount,date_bid=excluded.date_bid,bidder_id=excluded.bidder_id;
-- name: ContractDetailStats :one
SELECT count(*) FILTER(WHERE d.state IN ('pending','running'))::integer AS pending,
count(*) FILTER(WHERE d.state IN ('blocked','failed'))::integer AS failed
FROM eve_contract_details d JOIN eve_contracts c USING(owner_kind,owner_id,contract_id)
WHERE d.target_id=$1 AND c.in_scope;
-- name: DeletePersonalContracts :exec
DELETE FROM eve_contracts WHERE owner_kind='character' AND owner_id=$1;
