# Courier service

The /courier page implements the HA Logistics workflow requested from glory-navy revision 955c2ded. Live routes provide origin, destination, transport type, rates, minimum reward, volume limit, expiration and completion days. Paused routes are unavailable; no example rates become production defaults.

Paste items for HA Logistics appraisal (volume and sell value as collateral), optionally add cargo, or enter all volume and collateral manually. Editing the list invalidates its previous appraisal. Unrecognized items, invalid numbers, unavailable data and oversized cargo prevent contract copying.

Reward = max(volume × rate per m³ + collateral × percentage / 100, minimum reward). Display uses compact M/B/T amounts; copied reward and collateral are rounded to integer ISK without grouping. Copy the recipient Heavy Assets Logistics, destination, reward, collateral and route deadlines into an in-game courier contract. This page does not create contracts, orders, payments or approvals.

GET /api/v1/courier/routes requires a session and profile gate. Routes are cached for 300 seconds and returned with fetched_at; expired data is not served as a fresh quote after an upstream failure. POST /api/v1/courier/appraisal additionally requires Origin/CSRF and accepts appraisal_text up to 100000 characters / 5000 lines, with a 1 MiB request limit. Only the submitted list is forwarded to the fixed HA Logistics HTTPS endpoint, using an anonymous upstream-only XSRF session reused for at most five minutes, bounded by its cookie lifetime. Site cookies, user identities and EVE tokens are never forwarded. Responses validate prices, volume, missing items and optional HTTPS links on janice.e-351.com. Redirects are refused, responses and request duration are bounded, and two appraisals may run concurrently. Errors: 400 invalid input, 401 anonymous, 403 gate/CSRF, 429 busy (Retry-After: 5), 503 upstream unavailable. Upstream error bodies are not exposed.

Enable courier in the existing MODULES configuration while preserving identity/access and their dependencies, and deploy both frontend and backend. Remove it and restart to disable. No migration, credential, ESI scope, private store, River task, account merge data or delegable management permission is added. courier.self is only an authenticated entry capability.

If direct access is unavailable, COURIER_HTTP_PROXY accepts a trusted HTTP(S) CONNECT proxy (local development example: http://127.0.0.1:7890). An empty setting retains the standard outbound environment. This override affects courier only, never ESI/SSO. Invalid addresses fail closed without a direct fallback or logging credentials. The complete appraisal request times out after 28 seconds, within the existing 30-second reverse-proxy limit.

The implementation is independent of the reference source; see [notices](../third-party-notices.md). Third-party availability and actual game transport remain external. Verification and release status are maintained in [project status](../project-status.md).

## Appraisal performance (2026-10-23)

Complete, validated results without unrecognized items are cached in memory for at most 60 seconds, capped at 128 entries. Keys are SHA-256 hashes of the outer-trimmed list; raw lists are not retained or persisted. Quantity/content changes use different keys. from_cache includes either local or upstream cache hits. Expired, invalid and partial results are never reused as fresh data.

Concurrent identical lists share one bounded upstream operation; distinct requests retain the two-request limit. A disconnected reader does not cancel other readers, and the operation remains bounded to 28 seconds. A definitive upstream 403 invalidates the anonymous XSRF session for the next request, without replaying a POST or unknown outcome. Only homepage headers are needed, not its body. First requests and expired entries still depend on external HA Logistics / Janice latency.
