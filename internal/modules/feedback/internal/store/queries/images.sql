-- name: InsertImage :one
INSERT INTO feedback_images(ticket_id,message_id,ordinal,mime,content,digest,size_bytes)
VALUES($1,$2,$3,$4,''::bytea,$5,$6) RETURNING id;

-- name: InsertImageChunk :exec
INSERT INTO feedback_image_chunks(image_id,ordinal,content) VALUES($1,$2,$3);

-- name: ReadImageChunk :one
SELECT content FROM feedback_image_chunks WHERE image_id=$1 AND ordinal=$2;

-- name: ListImages :many
SELECT id,message_id,mime FROM feedback_images WHERE ticket_id=$1 ORDER BY ordinal,id;

-- name: ImageDigests :many
SELECT digest FROM feedback_images WHERE ticket_id=$1 AND message_id IS NOT DISTINCT FROM sqlc.narg(message_id)::bigint ORDER BY ordinal;

-- name: ReadImage :one
SELECT i.mime,i.content,i.size_bytes FROM feedback_images i JOIN feedback_tickets t ON t.id=i.ticket_id
WHERE i.ticket_id=$1 AND i.id=$2 AND (t.account_id=sqlc.arg(actor)::uuid OR sqlc.arg(administrator)::boolean);
