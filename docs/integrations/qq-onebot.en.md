# NapCatQQ integration

Updated 2026-10-17. [中文](qq-onebot.zh-CN.md). The user explicitly requested the third-party switch, superseding the previous official-only rule. See [project status](../project-status.md) for deployment and live acceptance.

## Selection and scope

[NapCatQQ](https://github.com/NapNeko/NapCatQQ) had 11,007 GitHub stars when checked on 2026-10-17. It runs as a separate, unmodified service, connected through OneBot 11 HTTP. The [Compose example](../../deploy/compose.napcat.example.yaml) pins the official v4.18.37 image digest; source release v4.18.38 did not yet have a matching formal Docker tag. No NapCat source is vendored. Its license restricts commercial use; see [notices](../third-party-notices.md).

An ordinary QQ account scans the login QR and joins the target groups. Official AppID, App Secret and OpenID cannot be reused as this identity. This delivery implements group verification codes, group selection and an administrator mention test. Pending approval reminders are implemented; deployment and live acceptance are tracked in project status. Other business notifications and automatic group admission remain unimplemented.

## Configuration and binding

Administrators save the bot QQ number, API origin, access token, webhook secret and enabled state on `/community`. Both credentials require at least 32 non-whitespace characters, are encrypted and write-only; blank inputs retain existing values. Revision checks reject stale saves. Connect to verify the bot identity and online status, select actual joined groups, and save. Previously selected groups can be removed even if absent from the current group response.

Members save their numeric QQ on `/account`, generate a ten-minute one-use code, and send the whole code directly in an enabled group from that same QQ. Mentions and command prefixes are unnecessary. A verified administrator can use “Mention me” in a saved group; the endpoint accepts only a group ID, derives the recipient from the administrator's current binding, and sends fixed text. An API acknowledgement is not proof of correct client rendering.

The first OneBot configuration save selects this transport and disables official callbacks. Turning it off does not restore the official adapter. Old OpenID history remains, but cannot confirm a numeric QQ identity. Changing QQ or its revision invalidates proof. Account merging removes source OneBot bindings without copying community details or translating between QQ and OpenID.

## API and transaction boundary

`GET/PUT /api/v1/community/qq/onebot/settings`, `GET /status` and `POST /test` under the same prefix use `community.group.manage`; the host checks the current administrator flag on each request. Mutations require session, same-origin Origin and CSRF. Credentials never appear in responses or logs. HTTP clients allow HTTPS or loopback HTTP origins, refuse redirects, time out after eight seconds and do not retry ambiguous sends.

The public `/api/v1/community/qq/onebot/webhook` validates HMAC-SHA1 of the raw body with the NapCat HTTP client token (`X-Signature: sha1=<hex>`), expected self_id and timestamps within five minutes in the past or one minute in the future. Bodies are capped at 64 KiB. Only normal group messages containing a complete code in text segments are processed. Other chat, self/private/anonymous messages, mixed image/quote/mention segments and unconfigured groups are ignored. Sender QQ must equal the saved profile number.

The transaction rechecks configuration revision and group scope under a shared row lock. Host-injected identity.LockActiveAccounts fences merged accounts. Event and QQ locks serialize claims; code consumption, current profile proof, numeric binding and audit commit atomically. Identical retries return an empty object without another reply; conflicting bodies fail. Chat text is not retained: code attempts store event IDs and SHA-256 hashes. Replies use standard OneBot quick operations with `at` and `text` segments. If a reply is lost after commit, the site still shows the successful binding; retries do not rebind or duplicate the reply.

## Operations and rollback

Goose 91 adds module-private settings, groups and numeric binding tables, reusing existing challenges and audit. Credentials use the QQ configuration key derived from EVE_TOKEN_KEY. Down refuses when OneBot configuration or bindings exist. Do not downgrade to an application that ignores the new identity type; stop callbacks and review database/configuration backups and old confirmation state before recovery.

Use private persistent `qq/`, `config/` and `plugins/` directories writable by UID/GID 1000. Pre-create `config/webui.json` with a random token, host `0.0.0.0`, port 6099. In `config/onebot11.json`, configure `network.httpServers` with enable=true, host `0.0.0.0`, port 3000, token `<ONEBOT_ACCESS_TOKEN>`, messagePostFormat `array`, enableCors=false and enableWebsocket=false. Configure `network.httpClients` with enable=true, URL `https://<SITE_HOST>/api/v1/community/qq/onebot/webhook`, token `<WEBHOOK_SECRET>`, messagePostFormat `array`, reportSelfMessage=false. Leave other network arrays empty and debug=false.

Publish ports only on host loopback and access WebUI through SSH forwarding. No privileged mode, host network or Docker socket is needed. Compose caps logs at 3×10 MiB and disables core dumps. Verify login, online status and group list before enabling site configuration. Keep login caches, credentials, QR codes and screenshots outside the repository and in private backups. Removing the old transport does not delete its audit history.

References: [API](https://napcat.napneko.icu/develop/), [message segments](https://napneko.github.io/develop/msg), [Docker](https://github.com/NapNeko/NapCat-Docker).

## Pending approval mentions (Goose 92)

Enable “Approval reminders” per receiving group on `/community`. New and upgraded groups default off. Community, approval and their dependencies must be enabled. The first enabled scan includes existing pending items. Welfare applications/cancellations, exchange cancellations and corporation loans retain source-owned authorization. Both site administrators and authorized corporation reviewers can receive reminders, provided their current QQ version is verified through OneBot and they remain in that group. Own applications, unauthorized/resolved items, fulfillment and information requests are excluded; personal lending is outside the approval center.

The host injects approval.PendingNotificationKeys, reading all live source pages without the index or five-second scope cache. Partial failures never close receipts. Community stores only opaque references and delivery metadata, not applicant names, amounts, reasons, approval state or payments. Standard OneBot at/text segments show each reviewer's pending count and the site `/approvals` URL.

The shared River runtime registers a separate community queue. `community.approval-scan.v1` runs each minute and on startup; `community.approval-group.v1` delivers at most one batch per group per minute, with at most 20 reviewers. Remaining recipients stay unclaimed. A continuous pending phase is mentioned once; new items and cancellation phases create new references. A same-phase re-entry is notified only when a complete scan observed the intermediate absence; transitions entirely between scans are not guaranteed to trigger another reminder. Config saves or disable/re-enable do not erase deduplication.

Before sending, the worker rechecks bot identity/status, current group membership, live permissions/state, settings revision, binding and active accounts. Offline and pre-send failures can retry. A durable claim precedes the external send, protected by a per-group lock. OneBot has no send idempotency key: ambiguous responses, missing message IDs or process exit after claiming are never automatically resent. This can lose one reminder; the approval center remains authoritative. Delivery states are sending/sent/unknown; abandoned sending becomes unknown after five minutes. Sent means an API message ID, not client rendering or read confirmation.

Goose 92 owns community_approval_notices and community_approval_deliveries. Inactive receipts and delivery batches expire after 90 days; active pending receipts remain to prevent duplicates. Approval/business audits are untouched. Account merges cancel source receipts without moving recipients, retain delivery batches, and evaluate the target's own binding and permissions independently. Before rollback, disable reminders and cancel unfinished jobs of both new kinds. Down refuses while reminders are enabled or delivery records exist; retain receipts and do not let an older binary consume unknown jobs.

Membership compatibility: the v4.18.37 [list implementation](https://github.com/NapNeko/NapCatQQ/blob/v4.18.37/packages/napcat-onebot/action/group/GetGroupMemberList.ts) refreshes its cache asynchronously; live checks observed an empty list while the bot remained in a populated group. Candidate discovery uses the cache, treating an empty list as unavailable. Before claiming a send, get_group_member_info with no_cache=true must match both group_id and user_id; failure leaves receipts unclaimed. Restarting the bot can require a new QR login. Configure the confirmed account for auto login through the protected WebUI, and verify actual online status.
