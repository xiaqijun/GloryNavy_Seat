-- name: SourceRates :many
SELECT * FROM exchange_source_rates ORDER BY source_id;
-- name: ShareSourceRate :one
SELECT * FROM exchange_source_rates WHERE source_id=$1 FOR SHARE;
-- name: LockSourceRate :one
SELECT * FROM exchange_source_rates WHERE source_id=$1 FOR UPDATE;
-- name: SetSourceRate :exec
UPDATE exchange_source_rates SET minor_per_unit=$2,conversion_mode=$3,version=version+1 WHERE source_id=$1;
-- name: SourceAward :one
SELECT * FROM exchange_source_awards WHERE source_id=$1 AND reference=$2 FOR UPDATE;
-- name: SaveSourceAward :exec
INSERT INTO exchange_source_awards(source_id,reference,account_id,units,minor_per_unit,coins) VALUES($1,$2,$3,$4,$5,$6)
ON CONFLICT(source_id,reference) DO UPDATE SET units=excluded.units,minor_per_unit=excluded.minor_per_unit,coins=excluded.coins;
-- name: CoinEntry :exec
INSERT INTO exchange_coin_ledger(account_id,kind,reference,request_key,delta,reason) VALUES($1,$2,$3,$4,$5,$6);
-- name: CoinHistory :many
SELECT * FROM exchange_coin_ledger WHERE account_id=$1 AND id<sqlc.arg(before_id) ORDER BY id DESC LIMIT 31;
