-- name: ReadContractList :many
WITH search_terms AS MATERIALIZED (
 SELECT DISTINCT lower(CASE WHEN part ~ '^#[0-9]+$' THEN substr(part,2) ELSE part END) AS term
 FROM unnest(regexp_split_to_array(sqlc.arg(search)::text,'[[:space:]]+')) AS parts(part) WHERE part<>''
), search_types AS NOT MATERIALIZED (
 SELECT t.term,n.type_id FROM search_terms t
 JOIN eve_sde_type_names n ON n.type_id::text=t.term OR strpos(lower(n.name_zh),t.term)>0 OR strpos(lower(n.name_en),t.term)>0
 WHERE n.release_id=(SELECT release_id FROM eve_sde_active_names WHERE singleton)
 UNION ALL
 SELECT t.term,n.entity_id FROM search_terms t JOIN eve_entity_names n ON strpos(lower(n.name),t.term)>0
 WHERE n.category='inventory_type'
), search_entities AS NOT MATERIALIZED (
 SELECT t.term,n.entity_id FROM search_terms t JOIN eve_entity_names n ON strpos(lower(n.name),t.term)>0
 WHERE n.category IN ('character','corporation','alliance','station','solar_system')
 UNION ALL
 SELECT t.term,p.character_id FROM search_terms t JOIN eve_character_profiles p ON strpos(lower(p.name),t.term)>0
 UNION ALL
 SELECT t.term,p.corporation_id FROM search_terms t JOIN eve_role_snapshots p ON strpos(lower(p.corporation_name),t.term)>0
)
SELECT c.* FROM eve_contracts c
WHERE c.owner_kind=sqlc.arg(owner_kind) AND c.owner_id=sqlc.arg(owner_id)
AND c.in_scope AND c.contract_id<sqlc.arg(before_id)
AND (sqlc.arg(status)::text='' OR c.status=sqlc.arg(status))
AND (sqlc.arg(kind)::text='' OR c.contract_type=sqlc.arg(kind))
AND (sqlc.arg(search)::text='' OR NOT EXISTS (
 SELECT 1 FROM search_terms t WHERE
 strpos(lower(coalesce(c.payload->>'title','')),t.term)=0
 AND strpos(c.contract_id::text,t.term)=0
 AND (t.term<>'0' AND t.term=ANY(ARRAY[c.payload->>'issuer_id',c.payload->>'issuer_corporation_id',c.payload->>'assignee_id',c.payload->>'acceptor_id',c.payload->>'start_location_id',c.payload->>'end_location_id']::text[])) IS NOT TRUE
 AND NOT EXISTS (SELECT 1 FROM search_entities n WHERE n.term=t.term AND n.entity_id=ANY(ARRAY[c.payload->>'issuer_id',c.payload->>'issuer_corporation_id',c.payload->>'assignee_id',c.payload->>'acceptor_id',c.payload->>'start_location_id',c.payload->>'end_location_id']::bigint[]))
 AND NOT EXISTS (
  SELECT 1 FROM eve_contract_items i JOIN eve_contract_details d USING(owner_kind,owner_id,contract_id)
  WHERE i.owner_kind=c.owner_kind AND i.owner_id=c.owner_id AND i.contract_id=c.contract_id
  AND c.contract_type<>'courier' AND d.part='items' AND d.state='ready'
  AND (i.type_id::text=t.term OR EXISTS(SELECT 1 FROM search_types n WHERE n.term=t.term AND n.type_id=i.type_id))
 )
))
ORDER BY c.contract_id DESC LIMIT 26;
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
