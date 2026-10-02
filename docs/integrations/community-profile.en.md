# QQ and KOOK community details

> Status index (2026-09-23): this business capability is deployed; remaining extensions and live verification gaps are tracked in the [backlog](../backlog.md). See [current project status](../project-status.md) for versions. Dated/Goose introduction notes describe their historical stage, not today’s release state. Apply current repository migrations, not only the initial module version.

Updated: 2026-09-29. [中文](community-profile.zh-CN.md). Manual details, editing, the profile-completeness gate, official QQ group admission callbacks, administrator fallback sync and the administrator group-settings card are implemented locally. Live QQ open-platform verification and a KOOK bot remain pending; platform OAuth is not a confirmed requirement.

## 1. Usage and states

Enter the QQ number and KOOK nickname in Community details on `/account`; use the edit icon to change them. Details belong to the site user and are shared across viewed characters, main-character changes and alternate-character logins.

- QQ: local input rule of 5–12 ASCII digits, no leading zero; stored as a string.
- KOOK: 1–64 Unicode code points after trimming, without control characters; internal spaces and emoji are preserved.
- Both fields must be valid to save a complete profile. These are local validation rules, not proof of a QQ/KOOK account or nickname ownership.
- Saving leaves group/server confirmation pending. Matching numbers or nicknames never merge users. Unverified inputs are not unique identity evidence and have no global uniqueness constraint.

`complete` means both details are present. Each platform separately reports `unfilled`, `pending` or `confirmed`. Members cannot supply confirmation state. Changing a confirmed value invalidates only that platform's confirmation; an unchanged save preserves revisions and confirmations.

## 2. API and module boundaries

`community` depends on `identity` and owns private SQL/store code. `GET /api/v1/community/profile` and `PUT /api/v1/community/profile` only access the current session user, with no target user ID. Mutations require a same-origin Origin, a site session and `X-CSRF-Token`. Bodies are limited to 4096 bytes and unknown fields are rejected.

PUT accepts `qq_number`, `kook_name` and `version`. Revisions are decimal strings; an empty profile starts at `"0"`. Conflicting edits return 409. The page preserves the draft and offers explicit reload before retry. See [OpenAPI](../../api/openapi.yaml).

When community is enabled, the host gates protected business operations on profile completeness, including for site administrators. Incomplete profiles receive `403 profile_required`. Session/logout, character management, EVE linking/reauthorization, own authorization summary and community profile operations remain available. Pending membership confirmation does not prevent normal business authorization after details are complete. `/` is the public landing page; business pages such as `/system` retain host session gating. Anonymous operational metadata is tracked separately under SEC-01 in the backlog.

## 3. Storage, upgrade and confirmation extension

Migration `00008_community_profile.sql` adds:

- `community_profiles`: site-user primary key, both details, aggregate revision and per-platform revisions.
- `community_profile_events`: user, profile revision, changed platforms and timestamp; no historical QQ/nickname values copied into audit content.
- `community_confirmations`: user, platform, field revision, source, actor, event ID, confirmation and invalidation timestamps. Source/event IDs are unique; each user/platform has at most one non-invalidated record.

Profile changes, revision increments, confirmation invalidation and audit commit in one transaction under a profile-row lock. Confirmation reads also require the current field revision, so late old-revision records cannot confirm new details. Invalidated history is retained separately from active status.

Migration `00050_community_qq_bot.sql` adds `community_bot_events` for official QQ callback sources, IDs, raw-body hashes and receive times. It never stores a bot token or EVE credential. Whether production has applied it is defined by the Goose version in project status.

Migration `00051_community_qq_official.sql` adds one-time official QQ binding challenges and openid bindings. Official events expose an openid, not a numeric QQ number; the legacy C2C binding flow remains compatible, but group admission does not require a private message.

Migration `00052_community_qq_group_join.sql` adds group admission applications and group-scoped bindings. During migration, `QQ_BOT_GROUP_OPENIDS` can provide a bootstrap list of comma-separated bot-scoped Group OpenIDs (numeric QQ group numbers are not accepted); after Goose 53, administrators maintain the list from `/community`. After entering a QQ number in the site, a member receives a one-time eight-character code valid for 15 minutes and places it in the QQ join verification message or review answers. On `GROUP_JOIN_REQUEST`, the site approves only a configured group whose unexpired code matches. On `GROUP_MEMBER_ADD`, it binds the group-scoped `member_openid` to the site account and marks the current QQ detail confirmed. The QQ profile revision is frozen at application time, so an old request cannot confirm a later QQ value.

Migration `00053_community_qq_group_settings.sql` adds administrator-managed group settings and migration `00054_community_qq_bot_settings.sql` adds database-backed official Bot settings. Administrators can maintain the App ID, API base, group OpenIDs, labels and enabled state from `/community`; `/account` only maintains personal QQ/KOOK details. The App Secret is write-only and encrypted server-side, and is never returned. Saving applies the callback and fallback-sync configuration immediately. Before the first save, `QQ_BOT_APP_ID`, `QQ_BOT_APP_SECRET`, `QQ_BOT_API_BASE` and `QQ_BOT_GROUP_OPENIDS` are bootstrap fallbacks; once saved, database settings take precedence and can intentionally clear the old environment list.

The official callback is `PUBLIC_ORIGIN/api/v1/community/qq/official/webhook`. The server uses the current database settings to exchange credentials for a short-lived access token and verifies `X-Signature-Timestamp` plus `X-Signature-Ed25519` on normal events. The initial `op=13` callback validation request has no signature headers; the server returns an Ed25519 signature over `event_ts + plain_token`. Admission uses the official `GET /v2/groups/{group_openid}/join_request_list` pagination endpoint followed by `POST /v2/groups/{group_openid}/approval_join_request/{member_openid}` with `op=approve`. The platform does not expose a numeric QQ number; group and member OpenIDs are scoped to the bot. `POST /api/v1/community/qq/group/application` requires the site session and CSRF token; application lists, sync, Bot settings and group settings are administrator-only. The sync endpoint is an idempotent fallback when callbacks are delayed; failures and rate limits remain visible in application status. The API base defaults to `https://api.bot.qq.com`; a loopback base may be used for local tests. Business errors are checked from the JSON `code`/`err_code`, not only from the HTTP status. The database ciphertext key is derived from the server-only `EVE_TOKEN_KEY` and is never exposed to the frontend or logs.

Implementation references: [QQ Bot API v2](https://bot.q.qq.com/wiki/develop/api-v2/), [access token](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/access-token.html), [API call guide](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/api-call-guide.html), [event subscriptions](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/interface-framework/event-emit.html), [security and authorization](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/interface-framework/sign.html), [group join request event](https://bot.q.qq.com/wiki/develop/api-v2/autogen/event/group_join_request.html), [C2C message event](https://bot.q.qq.com/wiki/develop/api-v2/autogen/event/c2c_message_create.html), and [group-at message event](https://bot.q.qq.com/wiki/develop/api-v2/autogen/event/group_at_message_create.html).

A KOOK adapter and live QQ platform integration test remain pending. The platform does not receive EVE credentials or site user IDs.

Enabled modules follow the current configuration. Add community to existing explicit MODULES settings, run `npm run db:migrate`, then restart the API; `dev:external` migrates automatically. Goose 8 introduced the module, Goose 50/51 added official events, Goose 52 adds group admission, and Goose 53/54 add group and Bot settings; see project status for the current migration. The foundation marker remains 1. Keep the environment values during migration, then maintain them from `/community`; existing users fill in personal details on their next account visit.

## 4. Verification scope

Coverage includes validation, user-level sharing, platform-specific invalidation, unchanged saves, late stale confirmations, concurrent edits, unknown-field/CSRF/anonymous rejection, administrator profile gating, plus official Ed25519 verification, validation handshake, challenge expiry, openid conflicts, official business error codes, admission-code matching, duplicate events, profile-revision changes and group-member binding. Frontend coverage includes field errors, saves/edits, confirmation messaging, admission-code generation, service failures and revision-conflict recovery. Live QQ platform testing remains pending.

## Administrator reads

Administrators read QQ/KOOK values and confirmation states through /api/v1/access/members/{user}/data, composed via the host-injected community.Get service. No edit revisions, credentials or confirmation mutations are exposed. /community/profile still refers exclusively to the signed-in account and accepts no target user. See [member viewing](../ui/members.md).
