-- name: ESIBudgetUsage :one
SELECT coalesce(sum(amount),0)::bigint AS used,
 min(expires_at)::timestamptz AS next_expiry,
 max(expires_at)::timestamptz AS full_expiry
FROM eve_esi_charges WHERE limit_key=$1 AND expires_at>clock_timestamp() AND amount>0;

-- name: InsertESICharge :one
INSERT INTO eve_esi_charges(limit_key,amount,expires_at,settled) VALUES($1,$2,$3,$4) RETURNING id;

-- name: SettleESICharge :execrows
UPDATE eve_esi_charges SET limit_key=$2,amount=$3,expires_at=$4,settled=true WHERE id=$1 AND NOT settled;

-- name: ConfigureESIBudget :exec
UPDATE eve_esi_limits SET capacity=$2,window_seconds=$3,updated_at=clock_timestamp() WHERE limit_key=$1;

-- name: SnapshotESIBudget :exec
UPDATE eve_esi_limits l SET
 remaining=greatest(0,l.capacity-(SELECT coalesce(sum(c.amount),0) FROM eve_esi_charges c WHERE c.limit_key=l.limit_key AND c.expires_at>clock_timestamp())),
 reset_at=coalesce((SELECT max(c.expires_at) FROM eve_esi_charges c WHERE c.limit_key=l.limit_key AND c.amount>0 AND c.expires_at>clock_timestamp()),clock_timestamp()),
 updated_at=clock_timestamp() WHERE l.limit_key=$1;

-- name: CleanupESICharges :exec
DELETE FROM eve_esi_charges WHERE id IN (SELECT id FROM eve_esi_charges WHERE expires_at<clock_timestamp() ORDER BY expires_at LIMIT 10000);
