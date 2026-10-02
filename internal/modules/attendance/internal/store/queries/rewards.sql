-- name: LockPAPAccount :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(account_id)::text,740025));
