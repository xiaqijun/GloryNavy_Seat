# EVE Sentry warning-platform keys

Status: 2026-10-03. The first Seat-side slice, one-card rotation, automatic page preparation, and inline reveal/copy card interaction are deployed in production. The Sentry service on host 114, the `seat.kisectool.com` HTTPS proxy, and the Seat server credential passed no-token 401 and authorized protocol-validation probes. Sentry's M1 ordinary-client authentication slice remains disabled by default; a real member rotation still needs field acceptance. `/sentry` also has a first read-only alert Nutshell Coin ledger for the current member; charging-rule persistence is local in Goose 62 and production charging remains disabled.

When a member opens `/sentry`, the page automatically prepares one default key card. Each Seat account keeps one current key card; the page does not show use categories and provides no create or delete entry. The card uses a square layout, and the key area and copy button are always visible and clickable. After initial preparation or rotation, the full plaintext is shown directly in the current page memory; clicking the key area or copy button copies it. When plaintext is not available in the current session, the card stays masked and prompts the member to generate or rotate first; it never rotates automatically or tries to recover old plaintext. Rotation runs directly from the refresh button without a confirmation dialog. A successful rotation reuses the same local card with the new prefix; the new plaintext is shown only for that session. Plaintext is never stored in the database, browser storage, URL, or logs. Historical revoked rows remain in audit storage but are not repeated in the member list.

Seat API:

- `GET /api/v1/sentry/keys`: list the current account's keys.
- `POST /api/v1/sentry/keys`: create a key with `{name, permissions, request_key}`; permissions are `monitor` and/or `alert`, and `request_key` is a UUID idempotency key.
- `POST /api/v1/sentry/keys/{id}/rotate`: rotate the current key. Seat keeps the local card ID, creates a new remote key, and directly revokes the old remote key; a successful response contains a one-time `secret`. Name and uses are not changed by this operation.
- `DELETE /api/v1/sentry/keys/{id}`: revoke the corresponding remote key.
- `GET /api/v1/sentry/alert-pricing`: read the current alert price policy, including Nutshell Coin minor units, pricing seconds, grant cap/lifetime, and `charging_enabled`; seconds are not a balance.
- `PUT /api/v1/sentry/alert-pricing`: site administrators may save the price version, pricing unit, coin price, per-grant cap, and lifetime. The request carries the current `version`; conflicts return 409 and changes are audited. This endpoint does not enable charging.

The create endpoint returns a conflict when the account already has a non-revoked key. The page calls it automatically only when no current card exists; click the card's “Refresh key” button when a replacement is needed.

Creation records `creating` locally before directly calling the remote API. Only a remote response containing the matching `key_id`, protocol version 1, and `active` state changes the state to `active`. A network failure leaves `sync_error` and never reports success or silently creates another key; retrying the original `request_key` directly calls the idempotent remote create endpoint again. An unchanged retry replays the original operation with HTTP 200 and never returns the plaintext again; changed content conflicts. Rotation still creates the new remote key before revoking the old one; an unconfirmed result leaves a synchronization error for later handling. The revoke API remains available to protected server compatibility and audit flows, while the member page has no revoke button and audit history is retained.

Remote configuration is server-only:

```dotenv
SENTRY_INTEGRATION_URL=https://sentry.example.com
SENTRY_INTEGRATION_TOKEN=<server-only-token-at-least-32-chars>
```

EVE Sentry now exposes `POST /api/v1/integrations/seat/keys` and `DELETE /api/v1/integrations/seat/keys/{key_id}`. Calls use a dedicated Bearer credential and `Idempotency-Key`; Seat sends the operation ID, key/account IDs, key hash, prefix, permissions, and protocol version, never the plaintext secret. Sentry stores only the hash and handles retries by `operation_id`; identical content replays the record and changed content returns a conflict. Production uses `https://seat.kisectool.com` as the HTTPS origin and routes only this integration path to host 114; the service credential remains in restricted server environment files and is not documented, browser-visible, or placed in River payloads. Real member create/revoke and error-contract acceptance remain pending.

Sentry M1 adds an explicit one-to-one `auth_external_accounts` binding: a Seat `account_id` is not a Sentry local user ID and must be bound by a trusted management flow. `EVE_SENTRY_SERVER_SEAT_AUTH_MODE` defaults to `off`; `shadow` audits validation and denies requests, while `enforce` creates a business principal only for the `monitor`/`alert` endpoint allowlists. Unbound, revoked, disabled, or out-of-scope requests are rejected consistently. Seat has not enabled this mode, and production interoperability is not yet verified; a key being issued does not prove that a warning client can already connect.

This phase does not read screenshots, online time, quality scores, or alert events. Actual alert charging remains controlled by `SENTRY_ALERT_CONSUMPTION_ENABLED`, and the production switch remains off. Monitoring rewards and alert billing remain staged in the [EVE Sentry integration plan](../plans/eve-sentry-integration.zh-CN.md). Key issuance or price configuration being available does not mean the warning client or usage settlement is complete.
