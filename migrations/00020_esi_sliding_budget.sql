-- +goose Up
-- Short metadata transactions only; all workers must be stopped for upgrade.
CREATE TABLE eve_esi_charges (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 limit_key text NOT NULL REFERENCES eve_esi_limits(limit_key) ON DELETE CASCADE,
 amount bigint NOT NULL CHECK(amount >= 0),
 expires_at timestamptz NOT NULL,
 settled boolean NOT NULL DEFAULT false
);
CREATE INDEX eve_esi_charges_bucket_expiry ON eve_esi_charges(limit_key,expires_at);
CREATE INDEX eve_esi_charges_expiry ON eve_esi_charges(expires_at);
-- Preserve outstanding legacy debt until its conservative expiry, not a reset.
INSERT INTO eve_esi_charges(limit_key,amount,expires_at,settled)
SELECT limit_key,capacity-greatest(0,remaining),reset_at,true FROM eve_esi_limits
WHERE capacity>0 AND remaining<capacity AND reset_at>clock_timestamp();

-- +goose Down
-- Aggregate compatibility snapshots remain in eve_esi_limits for old binaries.
DROP TABLE eve_esi_charges;
