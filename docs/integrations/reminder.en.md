# Reminders and EVE Mail

The first event is pending approvals: actionable welfare requests/cancellations, exchange cancellations and corporation loans from live approval sources, excluding self-review. `/reminders` is under Administration; all settings, history and retries require current site administrator status. QQ remains disabled and is not used for recipient selection.

## Selectable events (Goose 98)

There are 20 categories, offered only when their source module is enabled. Upgrades do not select or send new categories. Existing sender consent and the master switch remain unchanged. Mail contains authorized business details, next actions and a page link, addressed to the recipient account's current main character.

| Category | Recipient and trigger |
| --- | --- |
| Pending approvals | Current source-authorized reviewers, excluding self-review |
| Welfare, redemption and loan progress | Owning account; latest actionable/approved/completed/rejected/cancelled stage; redemptions include fulfillment/cancellation |
| Contracts awaiting acceptance | Currently bound personal character explicitly named as assignee; outstanding, unexpired, current grant generation and successful sync within two hours; no public corporation contracts |
| Guarantees and collateral to handle | Invited/called/disputed guarantor, or borrower awaiting selected collateral delivery; terminal loans excluded |
| Loans awaiting funding; lending deposit received | Currently authorized pool manager, excluding own borrowing; lender with verified funding |
| Upcoming/overdue loan payments | Borrower of a funded loan, within three days of due date or unpaid past due; each installment/deadline independently deduplicated |
| PAP target and PAP changes | Last seven days of the UTC month, complete current-month alliance snapshot no older than 24 hours, below configured target; changes use the recipient's corporation PAP ledger. Unknown data is not zero |
| Guoke coin changes | Own new ledger entries, including awards, spending, refunds and reversals |
| Expired character authorization | Current binding/owner and reauthorization state, keyed by character/grant generation |
| Structure fuel/unanchoring | Current structure-page corporation permissions; 72 hours, 24 hours, exhausted or known unanchor date. POS uses existing estimates; fuel deadline day and severity suppress sampling jitter |
| Sync problems | Current site administrators only; current generation, at least three failures and last success older than one hour. Routine retries, reauthorization and obsolete generations excluded; a new incident after recovery is eligible |
| Warning service problems | Own Sentry key in sync_error for at least 15 minutes, keyed by that error-state update; no remote calls during notification scans |
| Feedback replies/updates and pending feedback | Owner receives another actor's reply/status update; current site administrators see other accounts' open feedback |

Selecting or re-enabling events establishes a start time. Progress and ledgers include only changes since activation and within 30 days; historical changes are not backfilled. Existing actionable work can notify on activation. Sources sample committed local facts each minute, so short-lived intermediate stages are not guaranteed. Categories deduplicate independently and rotate by least recent attempt, sharing one mail per recipient per five minutes. A failed source does not clear its receipts or prevent other categories from working.

Except reopened approval phases, the same event reference does not resend after temporary observation gaps, permission changes or disabling/re-enabling configuration. New installments, states, ledger entries, grant generations and recovered-then-failed incidents have distinct references. Recipient and source permissions are checked again at send time. SQL remains in each source's private store; reminders do not mutate business records, payments, currency or ESI snapshots.

Migration 98 adds event selection, activation timestamps and receipt category while preserving approval receipts. Deploy frontend and backend together. Down refuses new-category use. Before rolling back to an old sender, disable delivery and cancel unfinished reminder jobs; retain Goose 98 and all multi-category receipts. Old cross-category receipt reconciliation must not run.


## Business mail content

All 20 categories use ephemeral details read under the source module's current authorization. The subject is the category name. The Chinese body lists the item/reference, business fields, next action and page link. Existing sender and current-main-character routing are preserved; there is no manual per-category recipient picker.

| Business | Fields |
| --- | --- |
| Approvals | Request/cancellation type, reference, applicant, item, receiving character, request time and amount when its unit is known |
| Welfare and redemptions | Reference, project/reward, recipient, outcome, decision note and time; recorded ISK, Guoke coins and physical reward summary. Cancellations identify refunded coins |
| Loan progress | Reference, pool, principal, principal plus interest, decision note, update time and confirmed first due date |
| Upcoming/overdue payments | Loan, pool, installment, remaining principal/interest/total after partial repayments, and deadline |
| Guarantees/collateral | Borrower, guarantee amount and invitation/call/dispute; collateral delivery reference, source/recipient characters, item-kind count and contract instructions |
| Funding/contributions | Borrower, receiving character, pool, loan principal and review note; verified contribution amount, source character, contract and funding time. Funding links go to Loans, including personal pools |
| Personal contracts | ID, title, type, receiving character, price, reward, applicable collateral and expiration |
| PAP/coins | Event/character, signed change, reason and time; PAP target notices include month, current/required/missing points, UTC deadline and observation time |
| Authorization/structures/sync/Sentry | Character, structure/corporation/system or key name; authorization state, fuel/unanchor deadline, sync resource/failures/last success, controlled reason and action. No tokens, secrets or raw upstream errors |
| Feedback | Number, BUG/feature type, title, current state, author/content or responder/reply excerpt, recorded status change and time |

Money is formatted from integer minor units with two decimal places and thousands separators, preserving signs without floats or M/B/T abbreviations. Dates explicitly use UTC. Character names use local cached names, falling back to IDs when unavailable. Missing facts are not invented. User text is escaped; long remarks/replies end with an ellipsis and link to full details.

Each mail contains at most five items from one category, retaining five-minute account throttling and category rotation. Only listed items are claimed/notified; remaining items stay eligible for later batches. Escaped body length is bounded at 9,500 characters without cutting tags or the final link. Missing details, duplicate keys or detail-read errors fail closed before ESI, with a definite-failure record eligible for administrator retry.

The host injects `ReminderDetails`; source modules read private stores using their own authorized event references. Approval uses live sources; structures reuse their authorized snapshot reader. Content exists only in memory, never in reminder tables, job arguments or logs. After reading details, the sender, current recipient main, configuration revision and live source permissions/keys are rechecked before sending. Deduplication references, database schema, scopes and enablement are unchanged. Actual game-client rendering and receipt remain live acceptance items.

## Setup and consent

Enable `reminder` with `identity,access,eve,approval` in `MODULES`, apply Goose 93. Delivery defaults to disabled with no sender. Add `esi-mail.send_mail.v1` to the EVE developer application's allowed scopes. On Reminders, select your own bound character, choose Authorize mail, complete SSO using that same character, select categories, enable notifications and save.

Normal login keeps the existing 57 scopes (58 with fittings). Only the explicit `POST /api/v1/eve/characters/{id}/reauthorize?purpose=reminder-mail` flow adds the server-selected mail scope. Arbitrary browser scopes are not accepted. Start and callback recheck current administrator status, session, binding, expected character and the persisted requested scope snapshot. Authorization does not enable delivery. A later ordinary reauthorization may remove mail permission. Another administrator can pause the existing sender; enabling or replacing it requires an owned, authorized sender.

## Delivery boundary

The minute scanner queues one account ID per River job. Each mail goes to that account's current main character and contains authorized details for one category and the relevant page link. Corporation/alliance recipients and mailing lists are not used. The host injects live approval keys, identity projections and the token-free EVE mail gateway. Reminder storage never copies approval state or imports source stores.

The configured sender must remain bound to its original administrator account with matching owner and authorization generation. Immediately before sending, settings revision, recipient main character and live pending permissions are rechecked. A failed source read never clears receipts. ESI token rotation, generation checks and shared rate limits apply; mutation responses are not cached. `approved_cost=0` never authorizes a CSPA charge.

At most one attempt per recipient every five minutes. Existing pending phases are eligible when first enabled; unchanged phases are not repeated across configuration changes or restarts. A phase observed as ended and subsequently reopened becomes eligible again. Receipts are claimed durably before HTTP. Uncertain transport outcomes, 5xx, invalid success IDs and interrupted workers become `unknown` and are not retried automatically. `sending` older than five minutes becomes `unknown`. Only definite `failed` deliveries offer an administrator retry, subject to fresh checks and rate limits. `sent` means ESI returned a mail ID, not verified client receipt.

Remote HTTP and local configuration cannot be one atomic transaction. Final checks run before HTTP; already submitted requests cannot be recalled by disabling delivery.

## Storage, merge and rollback

Goose 93 owns settings, audit, receipt and delivery tables. Active receipts persist; inactive receipts and completed delivery history are retained for 90 days. Configuration/retry audit persists. The reminder queue has concurrency one. Disabled modules register handlers but no periodic scan; old jobs snooze.

The merge participant is always registered. Delivery history moves to the target while retaining the first `original_account_id`; receipts merge conservatively. A source account configured as sender is disabled, without transferring privileges or consent. Down refuses used history/audit or enabled settings. Before an old binary rollback, disable delivery, cancel unfinished `reminder.scan.v1` / `reminder.send.v1` jobs and remove the module from `MODULES`, preserving Goose 93 and receipts. After merged reminder data exists, do not run a binary lacking its merge participant.

See [API](../../api/openapi.yaml), [UI](../ui/reminder.md) and [status](../project-status.md). Official basis: [ESI specification, compatibility date 2026-08-18](https://esi.evetech.net/meta/openapi.json?compatibility_date=2026-08-18), mail POST scope `esi-mail.send_mail.v1`, shared `char-social` bucket of 600 tokens/15 minutes. Actual sender consent and client delivery remain REMINDER-01 acceptance items.

Authorization failures distinguish a mismatched character, expired session, missing mail permission and denied administrator access. A mismatch names the required sender. Callback diagnostics record only a fixed reason, request ID and expected/returned character IDs; they never record codes, tokens, cookies, full query strings or raw SSO responses. A callback does not by itself prove the grant was saved.
