# ESI synchronization operations

> Status index (2026-09-23): this business capability is deployed; remaining extensions and live verification gaps are tracked in the [backlog](../backlog.md). See [current project status](../project-status.md) for versions. Dated/Goose introduction notes describe their historical stage, not today’s release state. Apply current repository migrations, not only the initial module version.

Local fix pending release, 2026-09-24: if the database is unavailable when the API starts, River retries startup every 15 seconds until it succeeds or the process exits. After recovery, persisted due jobs resume; expired authorization facts are not extended and authorization, leases, and cache rules remain enforced. Older binaries need an API restart after database recovery. A “some approvals unavailable” message can reflect expired profile and role snapshots used by welfare approvals; check the `profile` and `authorization` targets and whether River is running.

Local recovery fix, 2026-09-19: the shared River client explicitly sets `RescueStuckJobsAfter` to three minutes instead of the previous approximately one-hour default. Orphaned running jobs become eligible for periodic rescue and normal retries; this is not a three-minute synchronization guarantee. River v0.47 also checks worker-specific timeouts, protecting the 16-minute SDE worker. Fences, idempotency and authorization remain enforced. Historical response_too_large errors cannot distinguish oversized bodies from the old interrupted-read misclassification. Upgrading does not mass-unblock targets; inspect and recover only affected targets, never missing-permission or revoked targets.

Wallet adds seven resources and `eve.wallet-resource.v1`, sharing the existing queue, cache, credential generation and lease/fence. Disable wallet to stop dispatch; cancel this job kind before an older binary rollback. See [wallet synchronization](wallet.en.md).


Local Goose 32 adds `killmails` (ship losses), dispatched while welfare is enabled through `eve.character-killmails.v1`. It shares character queues, cache/budget, grant generation and lease/fence checks. Discovery and pending references persist with per-detail publication; completed sweeps follow cache expiry. Sync status/manual-refresh contracts include the resource. See [character losses](character-losses.en.md) for ownership and rollback requirements. Existing attendance confirmation is unchanged; not deployed to production.

Goose 22 adds attendance evidence through the same River client, a two-worker attendance queue and a 30-second persistent-task dispatcher. Tasks do not enter eve_sync_targets or broaden eve.sync.manage private-data access. State/retry live in event details. Ship, assets and personal losses reuse shared cache, budgets and grant checks; see [attendance](attendance.en.md).

ESI requests now share a dedicated HTTP/cache/budget client and a token-free service entry; the old in-memory fallback is removed. `/sync` includes token expiry, refresh and request observation. See [client boundary and token observation](esi-client.en.md) for Goose 18, permissions, counters and failure semantics. Existing scheduling, cache and budget constraints remain in force.

Contract list completion and detail completion are separate: page-level progress is not a failure. Goose 16 repairs completed item state regression after grant/source changes; see the [contract guide](contracts.en.md). Unfetched items remain subject to the normal queue and rate limits.

Goose 17 restricts corporation business reads and detail synchronization to contracts involving the corporation itself. Unrelated alliance metadata is retained, but item/bid jobs stop and are omitted from pending/failed business counts. Old excluded jobs are safely cancelled. Upstream lists still complete every page; rate-limit budgets are not reset.

2026-09-14: phase one is implemented. See [中文](esi-sync.zh-CN.md), [plan v1](esi-sync-plan.en.md) and [SeAT research](seat-esi-sync.en.md). Production deployment and full live revocation recovery require separate acceptance.

## 1. Scope and entry points

The original resources are: `profile` stores public name, corporation and alliance; `authorization` aggregates affiliation, four corporation role groups and corporation name/CEO/alliance into authorization facts. Pages read local data. These facts neither assign in-game roles nor automatically grant site administration.

The character details card shows resource status, last success and icon actions. `/sync` provides name/character-ID search, status filtering, 30-item cursor pages, the latest 20 execution records and retry. Responses contain concise reason codes, never tokens or raw ESI bodies.

Ordinary members access only their linked characters. Administrators may GET status for other members' active bindings; self POST refresh retains ownership checks. Administration requires the separate global `eve.sync.manage` permission. `access.manage` alone does not grant it; site administrators retain global access. All mutations require server authorization, Origin and CSRF validation. The new permission is included in the delivered-feature catalog.

## 2. Scheduling and consistency

River 0.47.0 runs on PostgreSQL within the host: `eve_control` has one worker and scans every 30 seconds; `eve_characters` has two workers; the contract extension adds one `eve_contracts` worker. Dispatch also runs at startup. Persisted `next_due_at`, rather than the periodic trigger, owns business scheduling. Payloads contain only target IDs and authorization generations; job kinds have `.v1` suffixes. Modules remain trusted, compiled-in extensions.

Credential persistence, generation changes, target initialization and `InsertTx` share a transaction. Active jobs are deduplicated; completed jobs do not prevent future cycles. A target uses a 90-second lease, increasing fence, grant generation and completed job ID. Collection has a 55-second budget; River has a 60-second timeout. Fetches occur outside business write transactions; publication rechecks ownership, generation and lease. At-least-once execution is protected by business idempotency.

Shutdown stops intake, waits for workers, cancels on timeout, then closes the pool. River rescues interrupted jobs, and persisted due targets recover missed dispatch triggers. Reauthorization fences old jobs. Unlinking cascades credentials, targets, cache and snapshots; manual-action audit survives separately.

## 3. Cache, refresh and limits

- Public cache is shared by request; private cache is additionally isolated by character/generation and encrypted with `EVE_TOKEN_KEY`. Keys include compatibility date `2026-08-18`. Requests respect `Expires` and GET ETag/304.
- Attempt, success and content-change times are distinct. Identical 200/304 responses preserve content time. Cache hits never renew validation time. Authorization cache trust is limited to two hours, with a separate finite five-minute snapshot grace period.
- Token refresh locks and rereads the credential row within a ten-second budget. Rotated tokens commit before business ESI calls. A 401 allows at most one justified refresh; 403 blocks the resource without deleting refresh credentials. Confirmed invalid grants or lost scopes require reauthorization.
- Goose 20 uses a compatibility-specific catalog, persistent concurrent reservations, Used settlement and per-charge expiry. Remaining only tightens estimates; unknown costs remain reserved. 429 and legacy 420 respect retry/global protection, with 200ms pacing retained. See [client accounting and upgrade](esi-client.en.md).
- Network/5xx failures use exponential backoff and jitter. Five consecutive failures mark the target failed, followed by an hourly probe. Rate-limit and corporation-change waits do not consume the failure count. Manual refresh cannot bypass cache or rate-limit waits.
- Corporation changes invalidate previous role facts before publication. A persisted barrier waits out old cache validity; reauthorization cannot bypass it.

Responses are limited to 1 MiB. Cache writes serialize quota checks at 10,000 entries/64 MiB. Each dispatch cleans at most 500 items: cache one day after expiry, successful runs after 14 days, other finished runs after 30 days. River maintains its own job retention. Manual-action audit currently has no automatic deletion policy.

## 4. Configuration and migration

Reuse `.env` values for PostgreSQL, modules, EVE credentials, origin and encryption key. Configured EVE login requires a key whenever `eve` is enabled, even without `access`. Optionally set `ESI_USER_AGENT=GloryNavy/0.1.0 (contact: admin@example.com)` and replace the placeholder with a real operator contact before deployment. The default contains only application name/version. Never expose secrets through frontend configuration.

Upgrade: stop the old API/polling worker; back up the database and encryption key; run `npm run db:migrate`; build/start the new API; check worker availability and targets. The command runs Goose `00010_eve_sync.sql`, then `cmd/jobs-migrate` for River's independent migration line. API startup never migrates. Existing credentials receive two targets preserving their due times and revoked-authorization blocks.

For rollback, stop the new worker and retain additive tables before restoring a compatible application. Never run the old poller alongside River. Data restoration requires a consistent database/key backup and independent restore verification. Goose Down deletes sync data and is not the routine rollback path.

## 5. API and recovery

Prefix: `/api/v1/eve`. Full contracts: [OpenAPI](../../api/openapi.yaml).

| Method/path | Purpose |
| --- | --- |
| GET `/sync/characters/{id}` | Active character status for owner or site administrator |
| POST `/sync/characters/{id}/refresh` | Own enqueue; optional JSON `resource` |
| GET `/sync/targets` | Management query: `search`, `state`, `after` |
| GET `/sync/targets/{id}/runs` | Redacted run history |
| POST `/sync/targets/{id}/retry` | Management retry |

202 outcomes `queued`, `already_pending` and `deferred` never mean synchronization succeeded. Blocked targets return 409 and need consent, game-permission or identity recovery. An unavailable worker returns 503 for mutations and `available=false` in status responses. General readiness is not worker readiness.

Isolated real PostgreSQL tests cover rollback, duplicate dispatch, restart recovery, fencing, generation, concurrent refresh, post-rotation ESI failure, 403 credential preservation, corporation transitions, 304, shared limits, ownership and CSRF. Browser tests use fixtures for desktop/mobile behavior. If CCP rotates a token but the database commit outcome is lost, recovery cannot be guaranteed; retry or reauthorize according to the persisted credential.

## 6. Extensions

New EVE resources need versioned workers, required scopes, independent targets/storage, cache/retry policy and tests before exposing their feature permissions. Cross-module integration uses public injected services, never private stores. A future standalone worker can reuse assembly, but concurrency must be budgeted across instances.

Assets, wallets, full-corporation collection, reports, SDE import and a generic event bus are not delivered in this phase. SDE remains a separate implementation project.

## 7. Contract extension

Goose 11 adds personal/corporation lists, items and bids with durable pagination and independent detail state. Characters now have four sync resource types. Existing login scopes include both contract scopes; missing actual consent still requires reauthorization. Sync endpoints expose metadata rather than contract content. See the [contract guide](contracts.en.md) for source selection, TTLs, migration and limits.

## SDE automatic-update extension

Goose 15 adds a persistent SDE update clock using the existing 30-second control scan and River runtime. A separate `eve_sde` queue has one worker and a 15-minute work budget. Public SDE updates run without SSO; ESI business readiness still requires the authorization service. The default six-hour check downloads only higher builds, with persisted failure backoff and operator version pins. Business reads use the shared static-data service. See [SDE operations](sde-names.en.md). This supersedes earlier statements that SDE was unimplemented; assets, wallets and other planned business resources remain pending.

Goose 19 adds `/sync?view=rate-limits` for rate buckets and per-route consumption, distinct from login tokens. Actual X-Ratelimit-Used, remaining snapshots, unmeasured requests and local/upstream waits are recorded without changing scheduling or the limiter. See [semantics and retention](esi-client.en.md#esi-rate-buckets-and-endpoint-consumption-goose-19).

## Online resource (2026-09-15)

Enabling attendance adds `online` targets and `eve.character-online.v1` to the EVE character queue. Disabled modules do not dispatch new online work; existing workers snooze hourly. Dispatch seeds current grant generations and explicitly blocks missing `esi-location.read_online.v1`. Sampling reuses cache clocks, limits, and fencing; local cache hits never create fresh observations, while failures record null breaks. Retention, estimation, and older-binary rollback: [attendance guide](attendance.en.md).

## Fitting and skill resources (Goose 27)

Enabling fittings dispatches per-character fittings/skills jobs, kind `eve.fitting-resource.v1`. Complete snapshots replace prior data; successful empty lists remove deleted game fits. Shared Request caching, generation and publication fences remain authoritative; schedules follow upstream cache headers. Scopes, buckets, skill snapshot limitations and rollback are documented in the [fitting guide](fittings.en.md).

## Skills and queue synchronization

Goose 28 adds skillqueue. Either fittings or skills enables the one skills target; only skills enables the queue. Existing generation/fence/cache and rate observations apply without duplicate polling. See [skill management](skills.en.md).

## Training queue and deferred outcomes (2026-09-15)

Sync list/character responses now accept skillqueue and label it as the training queue. Manual refresh accepts the resource while preserving ownership/admin object checks, cache and budget constraints. The old frontend rejected a whole valid list containing this resource; OpenAPI enums are now aligned.

New runs waiting in deferred state for rate_limited, shared_source, authorization_pending or corporation_changed record outcome=deferred, not failed. Actual network/ESI faults remain failed. Failure counts, timing, cache and fences are unchanged. Historical rows are retained; UI reason labels distinguish waits and interrupted workers.

## Sync history retention performance (Goose 46, released 2026-09-23)

Maintenance remains on the 30-second control dispatch with one combined batch limit of 500 rows. Successful finished runs retain 14 days; other finished runs retain 30 days; unfinished runs remain. Goose 46 adds separate partial (finished_at,id) indexes for the two outcome branches. Each indexed branch selects at most 500 candidates, then a combined batch selects the oldest finish times with ID tie-breaking. The DELETE rechecks eligibility. Fences, cache retention and business audits are unchanged.

The migration uses nontransactional CREATE INDEX CONCURRENTLY. Retry drops/rebuilds these migration-owned names rather than skipping a potentially invalid index. Apply Goose 46 before releasing the query. Up deletes no history; Down only removes the indexes. Older applications remain compatible; keeping indexes on application rollback is recommended. The migration CLI currently times out after two minutes: inspect index validity and retry after failure.

See [database performance observation](../database-performance.md) for application-side slow query logging. An isolated 1.84M-row history with no expired rows took 394.107 ms for the old candidate SELECT and 0.475 ms for the new DELETE (zero rows deleted); this is a local benchmark, not a production guarantee.

Production verification on 2026-09-23: backend v0.1.0-sync-retention-observation-20260923, both indexes valid; the candidate SELECT executed in 1.059ms with no expired candidates. Control dispatch and synchronization continued. Database was not restarted.
