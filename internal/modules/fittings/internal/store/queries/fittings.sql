-- name: ListDrafts :many
SELECT id,name,version,updated_at FROM fittings_drafts WHERE account_id=$1 AND id<sqlc.arg(before_id) ORDER BY id DESC LIMIT 31;
-- name: GetDraft :one
SELECT * FROM fittings_drafts WHERE id=$1;
-- name: CreateDraft :one
INSERT INTO fittings_drafts(account_id,name,fit,request_key) VALUES($1,$2,$3,$4) ON CONFLICT(account_id,request_key) DO UPDATE SET request_key=excluded.request_key RETURNING *;
-- name: UpdateDraft :one
UPDATE fittings_drafts SET name=$3,fit=$4,version=version+1,updated_at=now() WHERE id=$1 AND account_id=$2 AND version=$5 RETURNING *;
-- name: DeleteDraft :execrows
DELETE FROM fittings_drafts WHERE id=$1 AND account_id=$2 AND version=$3;
