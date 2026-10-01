-- Module: eve. Contract history is independent of the upstream 30-day window.
-- +goose Up
ALTER TABLE eve_esi_cache ADD COLUMN page_count integer NOT NULL DEFAULT 0;
CREATE TABLE eve_contract_cursors (
 target_id bigint PRIMARY KEY REFERENCES eve_sync_targets(id) ON DELETE CASCADE,
 generation bigint NOT NULL, owner_id bigint NOT NULL, page integer NOT NULL DEFAULT 1,
 pages integer NOT NULL DEFAULT 0, expires_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE eve_contracts (
 owner_kind text NOT NULL CHECK(owner_kind IN ('character','corporation')),
 owner_id bigint NOT NULL, contract_id bigint NOT NULL,
 source_character_id bigint NOT NULL, source_generation bigint NOT NULL,
 contract_type text NOT NULL, status text NOT NULL,
 payload jsonb NOT NULL, checked_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(owner_kind,owner_id,contract_id)
);
CREATE TABLE eve_contract_details (
 owner_kind text NOT NULL, owner_id bigint NOT NULL, contract_id bigint NOT NULL,
 part text NOT NULL CHECK(part IN ('items','bids')),
 target_id bigint NOT NULL REFERENCES eve_sync_targets(id) ON DELETE CASCADE,
 source_character_id bigint NOT NULL, generation bigint NOT NULL,
 state text NOT NULL DEFAULT 'pending', reason text NOT NULL DEFAULT '',
 next_due_at timestamptz NOT NULL DEFAULT now(), page integer NOT NULL DEFAULT 1,
 pages integer NOT NULL DEFAULT 0, fence bigint NOT NULL DEFAULT 0,
 lease_until timestamptz, last_success_at timestamptz,
 PRIMARY KEY(owner_kind,owner_id,contract_id,part),
 FOREIGN KEY(owner_kind,owner_id,contract_id) REFERENCES eve_contracts ON DELETE CASCADE
);
CREATE INDEX eve_contract_details_target ON eve_contract_details(target_id,state);
CREATE TABLE eve_contract_items (
 owner_kind text NOT NULL, owner_id bigint NOT NULL, contract_id bigint NOT NULL,
 record_id bigint NOT NULL, type_id bigint NOT NULL, quantity bigint NOT NULL,
 is_included boolean NOT NULL, is_singleton boolean NOT NULL, raw_quantity bigint,
 PRIMARY KEY(owner_kind,owner_id,contract_id,record_id),
 FOREIGN KEY(owner_kind,owner_id,contract_id) REFERENCES eve_contracts ON DELETE CASCADE
);
CREATE TABLE eve_contract_bids (
 owner_kind text NOT NULL, owner_id bigint NOT NULL, contract_id bigint NOT NULL,
 bid_id bigint NOT NULL, bidder_id bigint NOT NULL, amount numeric NOT NULL,
 date_bid timestamptz NOT NULL,
 PRIMARY KEY(owner_kind,owner_id,contract_id,bid_id),
 FOREIGN KEY(owner_kind,owner_id,contract_id) REFERENCES eve_contracts ON DELETE CASCADE
);
INSERT INTO eve_sync_targets(character_id,resource,generation,display_name,state,reason)
SELECT c.character_id,r.resource,c.grant_generation,coalesce(p.name,''),
 CASE WHEN c.state='reauthorize' OR NOT r.scope=ANY(c.scopes) THEN 'blocked' ELSE 'idle' END,
 CASE WHEN c.state='reauthorize' THEN 'reauthorize' WHEN NOT r.scope=ANY(c.scopes) THEN 'missing_scope' ELSE '' END
FROM eve_credentials c LEFT JOIN eve_character_profiles p ON p.character_id=c.character_id
CROSS JOIN (VALUES ('character_contracts','esi-contracts.read_character_contracts.v1'),('corporation_contracts','esi-contracts.read_corporation_contracts.v1')) r(resource,scope);
-- +goose Down
DELETE FROM eve_sync_targets WHERE resource IN ('character_contracts','corporation_contracts');
DROP TABLE eve_contract_bids,eve_contract_items,eve_contract_details,eve_contracts,eve_contract_cursors;
ALTER TABLE eve_esi_cache DROP COLUMN page_count;
