# Member feedback

The authenticated `/feedback` page accepts bug reports and feature requests. Members see only their own tickets and can add follow-up details. Current site administrators can select All feedback, reply, change status or reopen tickets. Corporation administrators, specialist roles and `access.manage` do not grant this access.

Statuses are `submitted`, `in_progress`, `resolved`, and `closed`. Original submissions and message history cannot be edited or deleted. Author names, staff markers and timestamps are saved with each event. Content renders as plain text. Lists support kind/status filters and stable ID-descending pagination of 30 rows; protected detail links use `?ticket=<id>`. Screenshots are private; there is no public board. Feedback update/pending reminders follow the separately configured reminder module.

## Screenshots (Goose 99)

Submissions and replies accept optional `images: [{mime, content}]`, where content is base64 without a data-URL prefix. Each request allows 0–3 PNG/JPEG images, each at most 2,000,000 bytes (2 MB) and 16,000,000 pixels. Server-side signature, dimensions and complete decoding reject damaged, spoofed and oversized files; SVG/GIF/WebP are not accepted. The 8 MiB JSON cap also accommodates three maximum-size files. Text requirements remain unchanged.

The form supports file selection and clipboard paste, preview/removal, and preserves images on failed submissions. Reading or submitting disables repeat actions. Detail and reply history expose only optional `images: [{id, mime}]` metadata; lists and reminder mail omit image contents. `GET /api/v1/feedback/tickets/{id}/images/{image}` returns the binary original only for the current ticket owner or current site administrator, with `Cache-Control: private, no-store` and `X-Content-Type-Options: nosniff`. A wrong parent or inaccessible image returns 404. Thumbnails link to this authenticated original.

The module-private `feedback_images` table stores content and SHA-256 digests with ticket/message foreign keys and 1–3 ordinals. Parent writes and images share a transaction, including replies and revision updates; retries compare image bytes and order. Storage failures roll back the entire operation. Attachments have no independent owner: merging the parent changes image access immediately, keeps original author/history, and needs no extra merge participant. Goose 99 Down refuses populated attachments. Deploy both sides after migration; once images exist, do not roll back to a frontend/backend that ignores them. No production test submissions or mail are required.

## API

- `GET /api/v1/feedback/tickets`: `scope=mine` by default; `scope=all` requires current site administrator status. Optional `kind`, `status`, and positive decimal `before`. Returns `tickets`, `next_before`, `administrator`; list rows omit the description.
- `POST /api/v1/feedback/tickets`: nonzero UUID `request_key`, `kind=bug|feature`, `title` (1–120 characters), `body` (1–6000), trimmed. Returns the saved ticket.
- `GET /api/v1/feedback/tickets/{id}`: returns `ticket`, `messages`, `administrator`. Owner or current site administrator only; unrelated accounts receive the same 404 as missing records.
- `POST /api/v1/feedback/tickets/{id}/messages`: nonzero UUID `request_key`, original `revision`, `body` (up to 2000 characters), optional nonempty `status` for administrators only. Empty status keeps the current status; status-only events may omit message text. Conflicts return 409; refresh before retrying while retaining the draft.

Host session/CSRF protection uses `feedback.self`, which is not a delegated management permission. Object authorization is enforced server-side, with administrator status checked again before writes. Requests are limited to 8 MiB; unknown fields and trailing JSON are rejected. Errors include 400, 401, 403, 404, 409, 429 and 503. See [OpenAPI](../../api/openapi.yaml).

Submission and reply keys are idempotent for the original author. Reusing a key with different content returns 409. Replay lookup precedes revision checks. Rolling 24-hour limits: 20 new tickets per account and 100 replies per member (administrators exempt from reply daily limits). A ticket allows 100 messages/status events; further discussion can use a new ticket. Actor advisory locks serialize limits; row locks and revisions prevent lost updates.

## Storage, merging and deployment

Goose 96 introduces private `feedback_tickets` and `feedback_messages` tables, indexes and active-account triggers. Queries and generated code stay in this module's private store. The host injects current administrator checks, session author identity and `identity.LockActiveAccounts`; account locks precede module locks. No ESI requests, scopes, credentials or background jobs are added.

The merge participant is always registered even when the UI is disabled. Current ownership transfers; first `original_account_id`, submission authors, message authors and timestamps remain intact. Revisions increase on transfer. Preview fingerprints include both accounts' ticket IDs/revisions, invalidating old previews after replies/status changes. Administrator privileges are never transferred.

Deploy matching frontend/backend artifacts from clean, pushed main. Back up database/configuration, apply current Goose/River migrations, append `feedback` to existing `MODULES`, retaining identity/access dependencies. Disabling the module hides routes without removing history. Down refuses populated history; older programs lacking this merge participant are not safe rollback targets after submissions exist.

See [project status](../project-status.md) for release evidence and [page layout](../ui/feedback.md).
