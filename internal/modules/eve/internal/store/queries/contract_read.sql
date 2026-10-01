-- name: ReadContractList :many
SELECT * FROM eve_contracts
WHERE owner_kind=sqlc.arg(owner_kind) AND owner_id=sqlc.arg(owner_id)
AND in_scope
AND contract_id<sqlc.arg(before_id)
AND (sqlc.arg(status)::text='' OR status=sqlc.arg(status))
AND (sqlc.arg(kind)::text='' OR contract_type=sqlc.arg(kind))
AND (sqlc.arg(search)::text='' OR strpos(lower(coalesce(payload->>'title','')),lower(sqlc.arg(search)))>0 OR contract_id::text=sqlc.arg(search))
ORDER BY contract_id DESC LIMIT 26;
-- name: ReadContractCorporations :many
SELECT DISTINCT ON (corporation_id) corporation_id,corporation_name,alliance_id,ceo_id
FROM eve_role_snapshots WHERE valid_until>now() ORDER BY corporation_id,synced_at DESC;
-- name: ReadContractDetailStates :many
SELECT part,state,reason,last_success_at FROM eve_contract_details WHERE owner_kind=$1 AND owner_id=$2 AND contract_id=$3 ORDER BY part;
-- name: ReadContractItems :many
SELECT * FROM eve_contract_items WHERE owner_kind=$1 AND owner_id=$2 AND contract_id=$3 AND record_id>$4 ORDER BY record_id LIMIT 101;
-- name: ReadContractBids :many
SELECT owner_kind,owner_id,contract_id,bid_id,bidder_id,amount::text AS amount,date_bid FROM eve_contract_bids WHERE owner_kind=$1 AND owner_id=$2 AND contract_id=$3 AND bid_id<$4 ORDER BY bid_id DESC LIMIT 101;
-- name: ReadEntityNames :many
SELECT * FROM eve_entity_names WHERE entity_id=ANY(sqlc.arg(ids)::bigint[]) AND language=sqlc.arg(language) AND expires_at>now();
-- name: SaveEntityName :exec
INSERT INTO eve_entity_names(entity_id,name,category,language,expires_at) VALUES($1,$2,$3,$4,now()+interval '1 day')
ON CONFLICT(entity_id,language) DO UPDATE SET name=excluded.name,category=excluded.category,expires_at=excluded.expires_at;

-- name: ReadContractSummaryItems :many
WITH types AS (
 SELECT i.contract_id,i.is_included,i.type_id,sum(i.quantity)::text AS quantity,min(i.record_id) AS first_record
 FROM eve_contract_items i
 JOIN eve_contract_details d USING(owner_kind,owner_id,contract_id)
 WHERE i.owner_kind=sqlc.arg(owner_kind) AND i.owner_id=sqlc.arg(owner_id)
 AND i.contract_id=ANY(sqlc.arg(contract_ids)::bigint[]) AND d.part='items' AND d.state='ready'
 GROUP BY i.contract_id,i.is_included,i.type_id
), ranked AS (
 SELECT *,count(*) OVER(PARTITION BY contract_id,is_included) AS type_count,
 row_number() OVER(PARTITION BY contract_id,is_included ORDER BY first_record,type_id) AS position
 FROM types
)
SELECT contract_id,is_included,type_id,quantity,type_count FROM ranked WHERE position=1 ORDER BY contract_id,is_included DESC;
