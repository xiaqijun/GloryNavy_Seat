-- Actual ESI journals can contain multiple observations of a reference ID.
-- Keep distinct payloads instead of silently overwriting money evidence.
-- +goose Up
ALTER TABLE eve_wallet_observations ADD COLUMN entry_key text NOT NULL DEFAULT '';
UPDATE eve_wallet_observations SET entry_key=md5(payload::text) WHERE kind='journal';
ALTER TABLE eve_wallet_observations DROP CONSTRAINT eve_wallet_observations_pkey;
ALTER TABLE eve_wallet_observations ADD PRIMARY KEY(owner_kind,owner_id,division,kind,record_id,entry_key);
-- +goose Down
-- Refuse destructive collapse when several variants exist for one reference.
ALTER TABLE eve_wallet_observations DROP CONSTRAINT eve_wallet_observations_pkey;
ALTER TABLE eve_wallet_observations ADD PRIMARY KEY(owner_kind,owner_id,division,kind,record_id);
ALTER TABLE eve_wallet_observations DROP COLUMN entry_key;
