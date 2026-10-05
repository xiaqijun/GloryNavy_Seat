# EVE Sentry keys and time billing

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

New charges read only Sentry's `GET /api/v1/integrations/seat/client-usage`. Each row is a server-confirmed interval between adjacent valid authenticated heartbeats; Seat settles seconds multiplied by the hourly price idempotently. Event counts, deliveries, remote prepaid grants, releases, and refunds are not billing inputs; their compatibility endpoints and historical tables were removed. Seat retains only the atomic ledger reference needed to settle each interval.

Monitoring rewards read `GET /api/v1/integrations/seat/monitor-contributions`. Only the primary node of each system contributes valid online time. Multiple alert systems are metered independently and merged into the account-level coin ledger.

Service tokens remain server-side only and never enter the browser or documentation. The production origin remains `https://seat.kisectool.com`.
