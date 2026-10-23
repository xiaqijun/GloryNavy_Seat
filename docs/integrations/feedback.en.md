# Member feedback

## Complete workflow (Goose 105)

Submit → administrator accepts or declines → start handling → submit completion → submitter confirms. A non-owner site administrator can accept/decline submitted feedback, start accepted work, and submit a result from in_progress to awaiting_confirmation. Only the current owning account can confirm to resolved, or provide a reason and return it to in_progress. The owner may reopen resolved work or resubmit a closed ticket with a reason. Administrators cannot confirm for the owner or process their own ticket as staff.

Declining/closing, completion results, returning, reopening and resubmitting require a body. Normal replies leave state unchanged. Role, locked current state, revision and idempotency are enforced in the messages transaction. Acceptance still credits the fixed reward once; subsequent cycles and merges never repeat it. Historical resolved tickets are not backfilled into confirmation. Historical in_progress tickets without adoption can submit completion, or be explicitly accepted; no upgrade-time payment occurs.

The detail shows a progress indicator and permitted action buttons above the reply history, instead of an arbitrary status dropdown. Existing feedback updates notify the owner. A return from awaiting_confirmation creates a stable `returned:<status message ID>` administrator pending reference; comments do not change it. No reminder type is newly enabled. Retain the new state/history in both frontend and backend; Down rejects confirmation history and old code cannot safely read or process it.

## Adoption rewards (Goose 104)

Site administrators configure a fixed Goose Coin reward per accepted ticket; zero disables payment and is the default. Only a current site administrator can accept another account's ticket. First acceptance freezes amount, operator and time, atomically with history and the ledger. Credit failure rolls back acceptance. Each ticket can receive one reward for its lifetime, including concurrent retries, reopening and account merges. Zero also freezes the decision; configuration changes never backfill tickets. Resolving or closing a ticket does not accept it or reverse a reward.

GET/POST `/api/v1/feedback/reward-settings` require current site administrator access; POST also requires CSRF, version, UUID request_key and amount_minor (0–10^12, 0.01 coin units). Messages accept the new `accepted` status, never a client-selected amount. Ticket responses optionally include accepted_at and reward_minor. The host injects exchange.FeedbackGrantTx with the fixed `feedback_reward:<ticket ID>` source. Frozen adoption data follows the parent ticket on merge, while exchange merges ledger ownership. Configuration has its own audit and idempotency records. Retain schema, status handling and merge participants after adoption; Down or rollback to code ignoring reward claims is unsafe. No new ESI scope or worker; existing feedback and coin-change reminders apply.

The authenticated `/feedback` page accepts bug reports and feature requests. Members see their own tickets, reply and confirm or return completion. Site administrators can select All feedback and handle other members' tickets through the workflow above. Corporation administrators, specialist roles and `access.manage` do not grant this access.

Statuses are submitted, accepted, in_progress, awaiting_confirmation, resolved and closed. Original submissions and append-only history retain authors, staff flags and times. Content is plain text, private to the owner and administrators. Kind/status filters, 30-row ID-descending pages and protected detail links remain available.

## Screenshots (Goose 99/100)

Submissions and replies use multipart/form-data: one `payload` field with strict JSON business metadata (no images, at most 32 KiB), plus 0–3 PNG/JPEG file parts named `image`. There is no configured file-size, multipart-request-size or pixel-count ceiling. The legacy JSON/base64 transport retains its 8 MiB request protection; the new UI uses multipart whenever files are attached. Text, type, permission and three-file rules remain unchanged.

The browser retains File objects and uses revocable Blob URLs instead of base64 copies. Selection, paste, removal and failed drafts remain supported. After authentication, the server streams files to private temporary files, checks signatures/dimension metadata, PNG chunk CRC/end markers and JPEG marker framing/end markers, without allocating full-resolution pixel buffers or fully decompressing pixels. Success and failure both close/remove temporary files; production retains systemd PrivateTmp. Only screenshot multipart requests and image reads override ordinary total read/write deadlines; the edge retains 300-second inactivity timeouts.

Goose 100 records byte counts in `feedback_images`, retains old content and adds private `feedback_image_chunks` keyed by image/ordinal, each at most 1 MiB. Metadata, every chunk, parent/reply and revision share one transaction. Failures roll everything back; SHA-256 replay comparisons still cover all original bytes and image order. New and legacy images use current parent ownership/current administrator access; account merges need no extra participant or chunk copies. Details expose only `images: [{id,mime}]`; lists and reminder mail omit originals.

`GET /api/v1/feedback/tickets/{id}/images/{image}` streams chunks with MIME/Content-Length, private/no-store and nosniff. Wrong parent, missing or inaccessible images return 404. Thumbnails link to this endpoint. Goose 100 requires size_bytes, so old image writers are incompatible: do not just switch back to an old backend. Existing chunks prohibit rollback and Down refuses populated chunks. Review data and migrations before any old-program recovery; prefer a forward fix. Existing database backups include all chunks; there is no permanent external upload directory.

Deploy the dedicated `/api/v1/feedback/` location from `deploy/edge-api-proxy.conf`: client_max_body_size 0, request/response buffering off and longer inactivity timeouts. Other APIs retain their existing limits. Activate backend, validate/reload proxy config, then activate frontend. Do not create production test feedback or notifications.

## API

- `GET /api/v1/feedback/tickets`: `scope=mine` by default; `scope=all` requires current site administrator status. Optional `kind`, `status`, and positive decimal `before`. Returns `tickets`, `next_before`, `administrator`; list rows omit the description.
- `POST /api/v1/feedback/tickets`: nonzero UUID `request_key`, `kind=bug|feature`, `title` (1–120 characters), `body` (1–6000), trimmed. Returns the saved ticket.
- `GET /api/v1/feedback/tickets/{id}`: returns `ticket`, `messages`, `administrator`. Owner or current site administrator only; unrelated accounts receive the same 404 as missing records.
- `POST /api/v1/feedback/tickets/{id}/messages`: nonzero UUID `request_key`, original `revision`, `body` (up to 2000 characters), optional nonempty `status` for administrators only. Empty status keeps the current status; status-only events may omit message text. Conflicts return 409; refresh before retrying while retaining the draft.

Host session/CSRF protection uses `feedback.self`, which is not a delegated management permission. Object authorization is enforced server-side, with administrator status checked again before writes. Legacy JSON requests are limited to 8 MiB and multipart metadata to 32 KiB; unknown fields and trailing JSON are rejected. Errors include 400, 401, 403, 404, 409, 429 and 503. See [OpenAPI](../../api/openapi.yaml).

Submission and reply keys are idempotent for the original author. Reusing a key with different content returns 409. Replay lookup precedes revision checks. Rolling 24-hour limits: 20 new tickets per account and 100 replies per member (administrators exempt from reply daily limits). A ticket allows 100 messages/status events; further discussion can use a new ticket. Actor advisory locks serialize limits; row locks and revisions prevent lost updates.

## Storage, merging and deployment

Goose 96 introduces private `feedback_tickets` and `feedback_messages` tables, indexes and active-account triggers. Queries and generated code stay in this module's private store. The host injects current administrator checks, session author identity and `identity.LockActiveAccounts`; account locks precede module locks. No ESI requests, scopes, credentials or background jobs are added.

The merge participant is always registered even when the UI is disabled. Current ownership transfers; first `original_account_id`, submission authors, message authors and timestamps remain intact. Revisions increase on transfer. Preview fingerprints include both accounts' ticket IDs/revisions, invalidating old previews after replies/status changes. Administrator privileges are never transferred.

Deploy matching frontend/backend artifacts from clean, pushed main. Back up database/configuration, apply current Goose/River migrations, append `feedback` to existing `MODULES`, retaining identity/access dependencies. Disabling the module hides routes without removing history. Down refuses populated history; older programs lacking this merge participant are not safe rollback targets after submissions exist.

See [project status](../project-status.md) for release evidence and [page layout](../ui/feedback.md).
