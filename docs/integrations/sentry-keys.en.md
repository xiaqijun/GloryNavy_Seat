# EVE Sentry keys and time billing

The 2026-10-20 production audit found a billing failure: an outdated unique index blocks remote usage inserts, and per-interval upward rounding inflates accumulated charges. The contract below is not evidence of completed ledger acceptance. See the [read-only audit and repair boundaries](../history/sentry-billing-audit-20261020.md).

Seat owns key creation/rotation, account ownership, hourly prices, and the administrator charging switch. Plaintext is encrypted at rest with a key derived from the server `EVE_TOKEN_KEY` and returned only in the protected list response for the owning account; it is never stored in browser storage, URLs, or logs. The page exposes copy and rotate actions and never rotates automatically.

Keys created before this migration have no recoverable plaintext and require one manual update; updated keys can be copied at any time.

## API

- `GET/POST /api/v1/sentry/keys`: read or create the current account key.
- `POST /api/v1/sentry/keys/{id}/rotate`: manually update a key, store the encrypted plaintext, and return the full content; the list can return it for later copying.
- `GET/PUT /api/v1/sentry/time-pricing`: administrators read or save alert and monitoring reward prices (Nutshell Coin/hour) and the charging switch.
- `GET /api/v1/sentry/alert-usage` and `GET /api/v1/sentry/alert-consumptions`: read account-level balance, spending, and reward records.
- `GET /api/v1/sentry/monitor-rewards`: reads recent reward intervals and returns `total_minor` and `total_count` for all credited rewards; the list is limited to recent records.

Saving the switch synchronizes `PUT /api/v1/integrations/seat/alert-consumption` on Sentry. A failed remote update prevents the local switch from committing.

## Billing rules

The Goose 107 guards are implemented but not yet released, and charging remains disabled. A new settlement requires a canonical positive integer `system_id`; settled replays and evidence covered by legacy batches retain their prior behavior. This validates the ID format. Attribution still comes from upstream evidence and does not establish that the client selected that system.

Resuming billing uses the latest actual off-to-on transition in the pricing audit. Ordinary price edits do not reset this window. Only intervals wholly within the current enabled window can create new charges. Earlier unsettled, paused, and boundary-crossing intervals are recorded per usage in `sentry_client_usage_exclusions`, including account, raw interval/seconds/system, reason and enabling audit, before the cursor may advance; they are not back-charged. No new exclusions are written while charging is disabled. Missing boundaries, invalid systems, failed audit writes and changed evidence fail without advancing the cursor.

The gate, exclusion evidence and billing decision share the settlement transaction; disabling waits for an in-flight transaction. Identical settled or excluded evidence can replay, but excluded payloads cannot change and become billable. Account merges preserve the first `original_account_id` and include exclusions in the preview fingerprint. A previously reserved but unsettled legacy grant excluded by the window retains its original reservation; it is not automatically released. Production had no such outstanding reservations during this verification. Once exclusions exist, Goose 107 Down and programs that ignore the window or exclusions are unsupported.

From Goose 106, new client usage keeps persistent ceiling credit per account, solar system, frozen price version, and frozen rate. Each interval pays only the increase in the rounded cumulative total: sixty one-second intervals at 2 coins/hour cost 0.04 coins in total. A zero-minor interval still has settled evidence but no zero-value coin entry. Stable raw `usage_id` keys survive page changes and retries; grants, charges, coin entries, and rounding credit commit in one transaction. Insufficient funds or any write failure rolls everything back.

The credit table starts empty. Price changes use separate buckets; historical charges and balances are never recalculated, refunded, or backfilled. Raw evidence fully covered by a legacy settled batch is skipped on replay; partial overlaps are rejected. Explicit account merges transfer and sum credit with merge fingerprint protection; the combined credit offsets future usage rather than issuing an immediate refund. See [migration, verification, and rollback boundaries](../history/sentry-billing-rounding-20261024.md).

New charges read only Sentry's `GET /api/v1/integrations/seat/client-usage`. Each row is a server-confirmed interval between adjacent valid authenticated heartbeats; Seat settles seconds multiplied by the hourly price idempotently. Event counts, deliveries, remote prepaid grants, releases, and refunds are not billing inputs; their compatibility endpoints and historical tables were removed. Seat retains only the atomic ledger reference needed to settle each interval.

Monitoring rewards read `GET /api/v1/integrations/seat/monitor-contributions`. Only the primary node of each system contributes valid online time. Multiple alert systems are metered independently and merged into the account-level coin ledger.

Service tokens remain server-side only and never enter the browser or documentation. The current production origin is `https://seat.glorynavy.com`. Update `SENTRY_INTEGRATION_URL` when changing domains so service POST requests do not pass through the legacy redirect.
