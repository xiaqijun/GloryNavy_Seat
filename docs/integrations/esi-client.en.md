# ESI client boundary and token observation

> Status index (2026-09-23): this business capability is deployed; remaining extensions and live verification gaps are tracked in the [backlog](../backlog.md). See [current project status](../project-status.md) for versions. Dated/Goose introduction notes describe their historical stage, not today’s release state. Apply current repository migrations, not only the initial module version.

Local fix, 2026-09-19: interrupted or timed-out response-body reads return retryable `network_error`, even after HTTP 200, instead of permanent `response_too_large`. Partial responses are neither cached nor published. The 1 MiB limit still rejects genuinely oversized bodies. Cache expiry, authorization and rate budgets are unchanged.

2026-09-14. See [中文](esi-client.zh-CN.md) and the [sync runbook](esi-sync.en.md). This follows SeAT's separation of responsibilities without introducing PHP or a separately deployed gateway.

## Layers

SeAT separates [Eseye](https://github.com/eveseat/eseye), the [EsiClient contract](https://github.com/eveseat/services/blob/master/src/Contracts/EsiClient.php), and the [job base](https://github.com/eveseat/eveapi/blob/master/src/Jobs/EsiBase.php). Our corresponding boundaries are:

| Component | Responsibility |
| --- | --- |
| `eve.ESIService.Request` | Token-free public/character request entry; expiry, content time, trust deadline and page metadata |
| `AuthorizationService.currentToken` | Per-character row lock, grant/scope/identity checks, refresh and committed rotation |
| `eve/internal/esiclient` | Fixed ESI host, compatibility date, HTTP parsing, encrypted cache, ETag/304, shared budgets and request observation |
| `SyncService` and resource workers | River scheduling, business visibility, pagination, leases/fences and publication |

The old in-memory fallback request path is removed. Profiles, contracts and public names use the Request entry. Authorization snapshots combine public and private requests inside the gateway, validate credentials first and share its bounded refresh retry. SQL remains private to the eve module.

Consumers define the narrow `Request(context.Context, eve.ESIRequest, any) (eve.ESIResponse, error)` interface; the host can inject `AuthorizationService.ESI()`. Requests specify Method, Path, Body, CharacterID, Generation, Scopes, ExpectPages and LimitTrust. Public requests omit identity and scopes; private calls must supply all three, using a generation from a trusted character job. Only GET/POST relative paths on Tranquility ESI are supported.

The gateway does not grant site permissions. Callers still enforce active binding, target authorization, corporation source selection and business scope. Public cache is shared; private responses remain encrypted and isolated by character and grant generation. Reusing corporation business data never means mixing character tokens.

## Lifecycle

- Credential acquisition retains its 10-second row-lock budget and rechecks the stored token. Refresh occurs on demand within one minute of expiry, not proactively for unused characters.
- Rotated tokens, expiry metadata and refresh success commit together before ESI collection. Network collection stays outside the business publication transaction.
- An ESI 401 may trigger one refresh retry. A newer token already committed by another caller is reused; retries still obey shared budgets. A 403 records a resource denial without refreshing or erasing credentials.
- Failed refresh transactions are released before a separate, generation-fenced observation transaction with a two-second budget. Old failures cannot overwrite a new grant. Process crashes or continued database failure can prevent persistence; logs contain only character ID, generation, a reason code and persistence status.
- A refresh with an uncertain rotation commit is classified as `rotation_commit_uncertain`, never presented as confirmed success. Observation cannot recover a lost CCP rotation.

## Monitoring

`/sync?view=tokens` and GET `/api/v1/eve/sync/tokens`, GET `/api/v1/eve/sync/tokens/{id}/events` require current global `eve.sync.manage` or site administrator status. `access.manage` is insufficient. Reads never decrypt credentials, refresh tokens or call ESI.

The list contains 30 characters per page with name/ID and state filters. Expiry, latest successful refresh, scope count, HTTP attempts and cache hits are visible; expandable details show acquisition reuse, refresh successes/failures, last request/error, scopes and the latest 20 events. Foreground polling runs every 30 seconds.

| State | Meaning |
| --- | --- |
| `valid` | Locally known expiry is more than one minute away; this is not a live CCP revocation probe |
| `refresh_due` | Expired or expiring; refresh on next request, not necessarily a login problem |
| `refresh_failed` | The latest refresh failure has not recovered |
| `reauthorize` | Existing credential lifecycle has marked authorization unusable |
| `unknown` | A historical credential has not yet supplied expiry metadata |

`reuse_count` counts credential acquisitions that reuse an access token, including acquisitions for cache reads; it is not network volume. `network_requests` counts HTTP attempts including network failures; 304 is a network attempt. `cache_hits` counts fresh local responses. `rate_limit_waits` combines local deferrals with upstream 420/429 responses; these are not all CCP rejections and do not count as request failures. HTTP status 0 means no response, including cache/defer/network failures. Public requests do not use a character token and are excluded from character counters.

Counters run from observation start and reset on a new SSO grant. Events retain their generation across reauthorization. No token values, prefixes, fingerprints, authorization headers, raw error bodies, encrypted credentials or owner hashes are exposed. These are neither daily analytics nor whole-site traffic counters.

## Upgrade and retention

Goose `00018_eve_token_observation.sql` adds metadata/event tables. Run `npm run db:migrate` (Goose plus River) and deploy compatible API/frontend. No new settings, scopes or assignable permissions are required.

Migration seeds unknown metadata without decrypting or probing historical tokens. The next acquisition fills expiry; future grants and refreshes supply events. No historical successes are fabricated. Unlink cascades both tables. Each control scan deletes up to 500 events older than 30 days; current-grant counters remain until reauthorization or unlink.

Prefer application rollback while retaining tables; Down deletes observation data but not credentials. See Goose 20 below for per-request budget accounting. External alerts, Prometheus and automatic multi-character request distribution remain unimplemented.


## ESI rate buckets and endpoint consumption (Goose 19)

`/sync?view=rate-limits` is the rate-bucket tab. `/sync?view=tokens` is now labelled login tokens; it observes SSO credentials, not request-budget tokens.

Following the [official ESI rate-limit documentation](https://developers.eveonline.com/docs/services/esi/rate-limiting/), routes share a floating-window budget by group and caller identity. This deployment has one application and one outbound identity: private buckets are per character, public buckets per shared egress. Grant changes and access-token refresh do not reset a bucket. Multiple applications or egress identities require extending this partition key.

- Remaining is exclusively an `X-Ratelimit-Remaining` snapshot, not live CCP balance. Capacity/window prefer response metadata, with compatibility-specific OpenAPI fallback and explicit `policy_source`. Snapshots older than their window are marked stale.
- Total, latest and mean consumption use only valid `X-Ratelimit-Used`. The mean denominator includes only measured responses. Missing, negative or malformed values are unmeasured, never inferred from status or differences between remaining snapshots.
- Typical 2xx/3xx/4xx/5xx costs are 2/1/5/0, except 429; actual response headers govern displayed consumption. HTTP attempts include transport failures; fresh local cache hits incur no network request; 304 is a network response.
- Local waits and upstream 420/429 responses are separate counters. Local availability derives from unexpired per-request charges and reservations. Recovery means the next charge expiry, not a full refill or guaranteed CCP balance.
- ESI retry time uses a valid 429 `Retry-After` delta or HTTP date; absent headers remain unknown instead of being replaced by local minimum waits.
- Since Goose 20 only declared or explicitly observed groups create buckets. Routes with neither declaration nor group header do not create a bucket. Historical unclassified rows are hidden and stop accumulating; normal retention still applies. Legacy error and internal game limits remain possible.

GET `/api/v1/eve/sync/rate-limits` accepts `search` (group, character name/ID; up to 80 characters) and `after` (bucket ID), returning 30 buckets per page. GET `/api/v1/eve/sync/rate-limits/{id}/routes` returns 30 normalized route aggregates, with `after` containing the previous route cursor (up to 300 bytes). Both require current global `eve.sync.manage` or administrator status. Missing bucket: 404; invalid input: 400; storage failure: 503. Read-only monitoring neither calls CCP nor decrypts credentials or changes budgets.

Goose `00019_esi_rate_observation.sql` adds module-private bucket and route aggregates. A separate best-effort transaction, bounded to two seconds, records completed attempts; observation failure logs a sanitized message and does not retry successful business requests. Only normalized route templates are stored (numeric IDs replaced by `{id}`), without query strings, bodies, authorization headers, token material, source IP or application secrets. Out-of-order writes add counters without replacing newer response snapshots.

Counters accumulate from first observation, not within the current window, and survive SSO reauthorization. Each maintenance pass removes at most 100 buckets inactive for 30 days and cascades route counters; active buckets retain cumulative history. Reading monitoring does not extend retention. There is no historical backfill, forced refresh or probe. No scopes, configuration or assignable permissions are added. Run `npm run db:migrate`; rollback can retain the tables, while Down removes this observation history. See Goose 20 below for the updated limiter.

See the [official-spec review](esi-rate-limits.en.md) for verified windows and completed adaptations. Current declared windows are 15m, but runtime policy remains per group.

## Official catalog and per-request budget (Goose 20, 2026-09-15)

- Embedded `catalog.json` contains 233 method/path policies for compatibility date `2026-08-18`, including 46 groups and per-route cache TTLs. The generator validates version, group consistency and values, recording source and SHA256. No specification fetch runs on the request path. Run `node scripts/update-esi-catalog.mjs`, review changes, test and rebuild to update it.
- Contract list/items/bids share the same group budget. Characters are isolated; SSO generations and refreshes do not change buckets. The present partition assumes one application and one egress identity. Valid observed groups override the catalog; subsequent responses update bucket capacity/window.
- PostgreSQL `eve_esi_charges` stores reservations and expiry, never credentials. Short transactions lock global error protection before the bucket. HTTP runs outside transactions. A known bucket reserves five tokens, then replaces that charge with valid Used; completion is idempotent. Concurrent workers and restarts cannot bypass reservations.
- Each charge returns after settlement time plus that group's window; later requests cannot postpone earlier expiry. Missing responses/Used retain five conservatively, while monitoring remains unmeasured. Crash reservations last the window plus the maximum 30-second HTTP deadline; existing shorter HTTP timeouts still apply.
- Remaining only tightens availability: a lower upstream snapshot adds the missing debt for one window; stale higher snapshots cannot mint local tokens. Unattributed external usage lacks its original timestamps, so this is a conservative local estimate, not a live CCP balance guarantee. Maintenance deletes at most 10,000 expired charges per pass.
- Valid Retry-After no longer has an artificial 60-second floor. Legacy error Reset protects global egress independently. Invalid/missing timing falls back to one minute; existing 200ms pacing remains.
- Cache freshness is separate: HTTP headers take precedence, followed by catalog x-client-cache-ttl; unknown routes use five minutes. Age, expired Expires, no-cache and no-store are respected. ETag takes priority, with Last-Modified as a conditional fallback. Fresh local cache makes no reservation; 304 remains a metered network response.
- Management `policy_source` is response/openapi/unknown for capacity/window only. `local_recovery_at` is the next charge expiry, not full recovery. The UI uses quota bars and endpoint comparison, showing up to ten entries from the current page with full-page table access. Unknown measurements are not zero; no historical trend is fabricated.

Upgrade: stop all old API/workers, back up database and encryption configuration, run `npm run db:migrate` through `00020_esi_sliding_budget.sql` (including River), then start matching binaries. Outstanding legacy budget becomes one conservative debt retaining its original expiry; migration does not reset quota, change due jobs or proactively fetch ESI.

New code also maintains conservative aggregate snapshots for a stopped-service application rollback. Never run both algorithms concurrently. After running an older binary, stop its workers and wait for all old budget/protection windows to expire before returning to the new algorithm, or restore/migrate to convert legacy debt again. Do not reuse the stale ledger accumulated before rollback. Down removes the ledger but retains aggregate budgets; stop new workers first.

### Validation timestamps

ESIResponse now exposes `ValidatedAt` and `Cached`. A successful 200/304 represents a new validation; a local cache hit exposes its stored validation/update time with Cached=true. `ContentUpdatedAt` continues to describe content changes. [Online sampling](attendance.en.md) does not fabricate observations from cache reads.

## Fitting writes

Mutation is limited to authenticated POST /characters/{id}/fittings/. It shares budget reservation/settlement and observation, bypasses response cache reads, conditional requests and cache persistence, and requires 201 with a positive fitting_id. The fittings module persists uncertain outcomes and prevents blind retries. Ownership and write scope remain mandatory. See [fittings](fittings.en.md).
