# Reminders and EVE Mail

The first event is pending approvals: actionable welfare requests/cancellations, exchange cancellations and corporation loans from live approval sources, excluding self-review. `/reminders` is under Administration; all settings, history and retries require current site administrator status. QQ remains disabled and is not used for recipient selection.

## Setup and consent

Enable `reminder` with `identity,access,eve,approval` in `MODULES`, apply Goose 93. Delivery defaults to disabled with no sender. Add `esi-mail.send_mail.v1` to the EVE developer application's allowed scopes. On Reminders, select your own bound character, choose Authorize mail, complete SSO using that same character, then enable pending approval reminders and save.

Normal login keeps the existing 57 scopes (58 with fittings). Only the explicit `POST /api/v1/eve/characters/{id}/reauthorize?purpose=reminder-mail` flow adds the server-selected mail scope. Arbitrary browser scopes are not accepted. Start and callback recheck current administrator status, session, binding, expected character and the persisted requested scope snapshot. Authorization does not enable delivery. A later ordinary reauthorization may remove mail permission. Another administrator can pause the existing sender; enabling or replacing it requires an owned, authorized sender.

## Delivery boundary

The minute scanner queues one account ID per River job. Each mail goes to that account's current main character and contains only its current pending count and an approvals link. No business details, corporation/alliance recipients or mailing lists are included. The host injects live approval keys, identity projections and the token-free EVE mail gateway. Reminder storage never copies approval state or imports source stores.

The configured sender must remain bound to its original administrator account with matching owner and authorization generation. Immediately before sending, settings revision, recipient main character and live pending permissions are rechecked. A failed source read never clears receipts. ESI token rotation, generation checks and shared rate limits apply; mutation responses are not cached. `approved_cost=0` never authorizes a CSPA charge.

At most one attempt per recipient every five minutes. Existing pending phases are eligible when first enabled; unchanged phases are not repeated across configuration changes or restarts. A phase observed as ended and subsequently reopened becomes eligible again. Receipts are claimed durably before HTTP. Uncertain transport outcomes, 5xx, invalid success IDs and interrupted workers become `unknown` and are not retried automatically. `sending` older than five minutes becomes `unknown`. Only definite `failed` deliveries offer an administrator retry, subject to fresh checks and rate limits. `sent` means ESI returned a mail ID, not verified client receipt.

Remote HTTP and local configuration cannot be one atomic transaction. Final checks run before HTTP; already submitted requests cannot be recalled by disabling delivery.

## Storage, merge and rollback

Goose 93 owns settings, audit, receipt and delivery tables. Active receipts persist; inactive receipts and completed delivery history are retained for 90 days. Configuration/retry audit persists. The reminder queue has concurrency one. Disabled modules register handlers but no periodic scan; old jobs snooze.

The merge participant is always registered. Delivery history moves to the target while retaining the first `original_account_id`; receipts merge conservatively. A source account configured as sender is disabled, without transferring privileges or consent. Down refuses used history/audit or enabled settings. Before an old binary rollback, disable delivery, cancel unfinished `reminder.scan.v1` / `reminder.send.v1` jobs and remove the module from `MODULES`, preserving Goose 93 and receipts. After merged reminder data exists, do not run a binary lacking its merge participant.

See [API](../../api/openapi.yaml), [UI](../ui/reminder.md) and [status](../project-status.md). Official basis: [ESI specification, compatibility date 2026-08-18](https://esi.evetech.net/meta/openapi.json?compatibility_date=2026-08-18), mail POST scope `esi-mail.send_mail.v1`, shared `char-social` bucket of 600 tokens/15 minutes. Actual sender consent and client delivery remain REMINDER-01 acceptance items.

Authorization failures distinguish a mismatched character, expired session, missing mail permission and denied administrator access. A mismatch names the required sender. Callback diagnostics record only a fixed reason, request ID and expected/returned character IDs; they never record codes, tokens, cookies, full query strings or raw SSO responses. A callback does not by itself prove the grant was saved.
