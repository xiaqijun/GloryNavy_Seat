# Official QQ bot

Updated 2026-10-28. [中文](qq-official.zh-CN.md). The user requested a switch from NapCat back to the official Bot API. This change is deployed with the official bot disabled by default; see [project status](../project-status.md).

## Configuration and binding

Administrators configure AppID, API origin, write-only App Secret, enabled state and callback URL at `/community`. Production API: `https://api.bot.qq.com`. The callback is `https://<SITE_HOST>/api/v1/community/qq/official/webhook`, displayed using the current site origin. Permissions and event subscriptions must be granted to the application. Standalone group codes require `GROUP_MESSAGE_CREATE`; group mentions and direct messages remain supported. A successful handshake does not establish permission for proactive or Markdown messages.

Secrets remain encrypted and are never returned or logged. An empty secret retains the saved value only for the same AppID. Disabling retains credentials; enabling a changed AppID requires a new secret. Writes require the current revision; 409 preserves the draft until an explicit reload. Changing AppID invalidates QQ confirmations and unused codes and removes current official OpenID mappings, retaining historical events/confirmations and OneBot evidence. Rebinding is required. Group OpenIDs are application-scoped and cannot be converted from numeric group IDs. Historical admission endpoints require actual platform permissions/events and separate live validation.

Members enter their QQ contact and generate a single-use code on their account page, valid for ten minutes. Send the complete eight-character code to the bot. Ordinary group messages can be used without mentioning the bot when that event is granted and subscribed. Unrelated group chat is ignored without content storage or replies. The legacy mention form is `@bot 绑定 CODE`; direct messages also remain supported.

Official callbacks expose OpenIDs, not numeric QQ numbers. Binding proves the relationship between the code holder's official bot identity and the site account; it does not verify the manually supplied number. Historical OneBot numeric bindings do not become official proofs. Changing contact details invalidates old confirmation.

Publication rechecks the enabled configuration and exact verified AppID/secret/API snapshot under a shared configuration lock. Event/identity locks serialize replay and ownership, and the host's active-account lock blocks late writes after account merges. Codes, versions, identity ownership, confirmation and event hashes commit atomically. Passive replies use the source message ID and are not automatically retried.

## Migration and compatibility

Goose 109 adds enabled (default false) and revision to official settings. It preserves old credentials without enabling them, disables OneBot and its approval-group switches, and invalidates OneBot confirmations and unused codes. Numeric bindings, groups, events and delivery receipts remain as history. Retired management routes retain session/current-administrator/CSRF checks and return 410; the old public webhook also returns 410. Old pages cannot reactivate the transport.

Historical queue handlers remain registered, but disabled OneBot settings prevent scanning or sending approval messages. QQ reminders are not automatically resumed, and game-mail reminders remain unchanged. Proactive group messages, Markdown member mentions and client rendering require permission-aware follow-up work; prior NapCat acceptance is not official-bot acceptance.

Down rejects configured official installations or official bindings; only blank unused development installations may remove the new fields. Do not run older code that ignores configuration versions, identity sources or receipts. This change does not log into a personal QQ account, start NapCat or delete history.

References: [access credentials](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/access-token.html), [message rules](https://bot.q.qq.com/wiki/develop/api-v2/server-inter/message/overview.html). Token lifetimes accept numeric and string values; string `7200` no longer falls back to 60 seconds. Credential/cache snapshots change together; HTTP redirects are refused.

Unsigned validation accepts a canonical positive decimal timestamp and a nonempty challenge up to 1024 bytes, rejecting challenges starting with a JSON object. It cannot be used to sign normal event JSON. Callbacks must contain exactly one JSON document.
