-- name: LockAccount :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(account_id)::text,740026));
-- name: ShopSettings :one
SELECT * FROM exchange_shop_settings WHERE singleton;
-- name: LockShopSettings :one
SELECT * FROM exchange_shop_settings WHERE singleton FOR UPDATE;
-- name: ShareShopSettings :one
SELECT * FROM exchange_shop_settings WHERE singleton FOR SHARE;
-- name: SetShopRate :exec
UPDATE exchange_shop_settings SET isk_per_coin=$1,version=version+1 WHERE singleton;
-- name: ListRewards :many
SELECT * FROM exchange_rewards WHERE NOT archived AND id>sqlc.arg(after_id) AND (enabled OR sqlc.arg(admin)::boolean) ORDER BY id LIMIT 31;
-- name: LockReward :one
SELECT * FROM exchange_rewards WHERE id=$1 FOR UPDATE;
-- name: SaveReward :one
INSERT INTO exchange_rewards(type_id,quantity,isk_value,stock,enabled) VALUES($1,$2,$3,$4,$5) RETURNING *;
-- name: UpdateReward :one
UPDATE exchange_rewards SET type_id=$2,quantity=$3,isk_value=$4,stock=$5,enabled=$6,version=version+1 WHERE id=$1 RETURNING *;
-- name: ChangeRewardStock :exec
UPDATE exchange_rewards SET stock=stock+sqlc.arg(delta)::integer,version=version+1 WHERE id=$1;
-- name: ShopBalance :one
SELECT coalesce((SELECT sum(a.delta) FROM exchange_coin_ledger a WHERE a.account_id=sqlc.arg(account_id)::uuid AND a.kind='source'),0)::bigint AS earned,
coalesce((SELECT sum(r.coins_minor) FROM exchange_redemptions r WHERE r.account_id=sqlc.arg(account_id)::uuid AND r.state IN ('pending','cancel_requested')),0)::bigint +
coalesce((SELECT sum(g.reserved_minor-g.settled_minor-g.released_minor) FROM exchange_alert_grants g WHERE g.account_id=sqlc.arg(account_id)::uuid AND g.state='active'),0)::bigint AS reserved,
coalesce((SELECT sum(r.coins_minor) FROM exchange_redemptions r WHERE r.account_id=sqlc.arg(account_id)::uuid AND r.state='fulfilled'),0)::bigint +
coalesce((SELECT sum(c.coins_minor) FROM exchange_alert_charges c WHERE c.account_id=sqlc.arg(account_id)::uuid AND c.state='settled'),0)::bigint AS spent;
-- name: FindRedemption :one
SELECT * FROM exchange_redemptions WHERE account_id=$1 AND request_key=$2;
-- name: CreateRedemption :one
INSERT INTO exchange_redemptions(account_id,request_key,fingerprint,reward_id,type_id,quantity,recipient_id,recipient_name,isk_per_coin,isk_value,coins_minor) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING *;
-- name: Redemption :one
SELECT * FROM exchange_redemptions WHERE id=$1;
-- name: LockRedemption :one
SELECT * FROM exchange_redemptions WHERE id=$1 FOR UPDATE;
-- name: DecideRedemption :exec
UPDATE exchange_redemptions SET state=$2,note=$3,decided_by=$4,decided_at=now(),version=version+1 WHERE id=$1;
-- name: ListRedemptions :many
SELECT * FROM exchange_redemptions WHERE id<sqlc.arg(before_id) AND (sqlc.arg(admin)::boolean OR account_id=sqlc.arg(account_id)::uuid) ORDER BY id DESC LIMIT 31;
-- name: FindShopAudit :one
SELECT * FROM exchange_shop_audit WHERE actor_id=$1 AND request_key=$2;
-- name: ShopAudit :exec
INSERT INTO exchange_shop_audit(actor_id,request_key,kind,target_id,fingerprint,payload) VALUES($1,$2,$3,$4,$5,$6);
