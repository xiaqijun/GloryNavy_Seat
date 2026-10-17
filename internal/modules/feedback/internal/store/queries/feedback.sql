-- name: ListTickets :many
SELECT to_jsonb(t)-'request_key'-'original_account_id'-'submitted_by'-'body' || jsonb_build_object('id',t.id::text) AS data
FROM feedback_tickets t
WHERE (sqlc.arg(all_accounts)::boolean OR account_id=sqlc.arg(actor)::uuid)
 AND (sqlc.arg(kind)::text='' OR kind=sqlc.arg(kind))
 AND (sqlc.arg(status)::text='' OR status=sqlc.arg(status))
 AND (sqlc.arg(before_id)::bigint=0 OR id<sqlc.arg(before_id))
ORDER BY id DESC LIMIT 31;

-- name: GetTicket :one
SELECT to_jsonb(t)||jsonb_build_object('id',t.id::text) AS data FROM feedback_tickets t WHERE id=$1;

-- name: LockTicket :one
SELECT id FROM feedback_tickets WHERE id=$1 FOR UPDATE;

-- name: FindSubmission :one
SELECT id FROM feedback_tickets WHERE submitted_by=sqlc.arg(actor)::uuid AND request_key=sqlc.arg(request_key)::uuid;

-- name: InsertTicket :one
INSERT INTO feedback_tickets(account_id,submitted_by,author_name,request_key,kind,title,body)
VALUES(sqlc.arg(actor)::uuid,sqlc.arg(actor)::uuid,sqlc.arg(author_name),sqlc.arg(request_key)::uuid,sqlc.arg(kind),sqlc.arg(title),sqlc.arg(body)) RETURNING id;

-- name: ListMessages :many
SELECT to_jsonb(m)-'request_key'-'actor' ||jsonb_build_object('id',m.id::text,'ticket_id',m.ticket_id::text) AS data
FROM feedback_messages m WHERE ticket_id=$1 ORDER BY id LIMIT 100;

-- name: FindMessage :one
SELECT ticket_id,body,status FROM feedback_messages WHERE actor=sqlc.arg(actor)::uuid AND request_key=sqlc.arg(request_key)::uuid;

-- name: InsertMessage :exec
INSERT INTO feedback_messages(ticket_id,actor,author_name,staff,request_key,body,status)
VALUES(sqlc.arg(ticket_id),sqlc.arg(actor)::uuid,sqlc.arg(author_name),sqlc.arg(staff),sqlc.arg(request_key)::uuid,sqlc.arg(body),sqlc.arg(status));

-- name: UpdateTicket :execrows
UPDATE feedback_tickets SET status=CASE WHEN sqlc.arg(status)::text='' THEN status ELSE sqlc.arg(status) END,revision=revision+1,updated_at=now()
WHERE id=sqlc.arg(id) AND revision=sqlc.arg(revision);

-- name: LockWriter :exec
SELECT pg_advisory_xact_lock(hashtextextended('feedback:'||sqlc.arg(actor)::text,0));

-- name: RecentSubmissions :one
SELECT count(*) FROM feedback_tickets WHERE submitted_by=sqlc.arg(actor)::uuid AND created_at>now()-interval '1 day';

-- name: RecentMessages :one
SELECT count(*) FROM feedback_messages WHERE actor=sqlc.arg(actor)::uuid AND created_at>now()-interval '1 day';

-- name: MessageCount :one
SELECT count(*) FROM feedback_messages WHERE ticket_id=$1;

-- name: MergeSummary :one
SELECT jsonb_build_object('tickets',count(*) FILTER(WHERE account_id=sqlc.arg(source)::uuid),
 'fingerprint',md5(coalesce(string_agg(id::text||':'||revision::text,',' ORDER BY id),''))) AS data
FROM feedback_tickets WHERE account_id IN(sqlc.arg(source)::uuid,sqlc.arg(target)::uuid);

-- name: MergeTickets :exec
UPDATE feedback_tickets SET original_account_id=coalesce(original_account_id,account_id),account_id=sqlc.arg(target)::uuid,revision=revision+1
WHERE account_id=sqlc.arg(source)::uuid;
