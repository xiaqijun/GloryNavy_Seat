# ESI synchronization implementation plan

> Baseline / historical research: recommendations here do not imply delivery. See the [module guides](README.md), [current status](../project-status.md) and [backlog](../backlog.md). Original upstream verification dates are retained.

> Implementation update: phase one is delivered. This document preserves plan v1; current behavior, migration, differences and limitations are in the [operations guide](esi-sync.en.md).

Date: 2026-09-14. Version: proposal v1. Status: **historical proposal; phase one delivered, current operations guide takes precedence**. [中文](esi-sync-plan.zh-CN.md)

Based on [SeAT research](seat-esi-sync.en.md), the [architecture](../architecture.md) and [technology baseline](../tech-stack.md). Technical parameters below require implementation validation.

## 1. Objectives and initial scope

Provide recoverable background synchronization, local data for pages and explicitly valid game-role facts for authorization.

Deliver persistent jobs; public profiles for linked characters; migration of affiliation, four role groups and basic corporation facts; per-resource status; self-service refresh; and an administrative status/retry view.

Assets, wallets, industry, contracts, mail, killmails and reporting datasets follow their business modules. SDE remains a separate versioned import and does not block this phase. Granted scopes do not automatically activate every dataset.

## 2. Architecture and extension

Use Go with River open-source core and PostgreSQL, retaining pgx/sqlc/Goose. River owns its migration lineage; Goose owns business migrations. API and workers initially share a process with explicit lifecycle control.

Flow: authorization or due-time scan → sync target → transactional River dispatch → resource worker → credential/cache/limit coordination → ESI → validated business commit → local views and access facts.

| Boundary | Responsibility |
| --- | --- |
| Proposed `internal/platform/jobs` | River setup, registration, enqueueing, lifecycle and basic instrumentation |
| `internal/modules/eve` | Credentials, ESI client, resources, scheduling, status and initial profile/authorization datasets |
| Future business modules | Resource definitions, mapping and idempotent writes through private stores |
| identity | Authoritative account-to-character bindings; synchronization cannot transfer ownership |
| access | Website permissions and consumption of current valid role DTOs |
| app | Explicit composition and startup/shutdown |

Resources declare ID/version, owner module, entity, required scopes, dependencies, schedule policy and worker. Payloads contain target IDs and generations, never credentials or full responses. Cross-module work uses injected business interfaces. Keep River types outside the pure module Manifest; register workers separately through the host. Runtime plugin installation is outside scope.

## 3. Initial jobs and scheduling

| Proposed kind | Work |
| --- | --- |
| `eve.sync-dispatch.v1` | Bounded scan of due targets; no ESI calls |
| `eve.character-profile.v1` | Public profile fields persisted by eve |
| `eve.character-authorization.v1` | Consistent affiliation, roles and corporation facts for access |

Keep authorization as one business commit, using shared public request caching internally. A new corporation in a public profile must never inherit old Director facts. Add corporation-keyed private datasets when their features are implemented.

Authorization/binding/reauthorization updates targets and generations and dispatches jobs in the credential transaction. Ordinary session login does not force a full refresh. Run a scanner at startup and, initially, every 30 seconds. Persist `next_due_at`; recover overdue work after downtime instead of replaying each missed interval.

Dispatch locks a target in a short transaction, enqueues with InsertTx and records its active job. Repeated requests reuse that job. Uniqueness explicitly covers active states including retryable and excludes completed; execution remains idempotent. Reconcile stale target references to terminal jobs without duplicating active work.

River's standard periodic schedule is in memory; use it only to wake the durable target scanner. [Periodic jobs](https://riverqueue.com/docs/periodic-jobs) Follow the supported transactional insertion interface. [Transactional enqueueing](https://riverqueue.com/docs/transactional-enqueueing) Unique insertion does not guarantee exactly-once work. [Unique jobs](https://riverqueue.com/docs/unique-jobs) No Pro workflow or durable-periodic feature is required.

Schedule from endpoint cache validity and retry constraints. For composite jobs, revalidate the earliest-due component while reusing other valid components without resetting their freshness. A proposed five-minute fallback applies when cache timing is unavailable. Add jitter; upstream wait times are lower bounds.

## 4. Data and state

These are logical structures, not published migrations:

| Structure | Purpose |
| --- | --- |
| `eve_sync_targets` | Unique environment/module/resource/entity; due time, active job, grant generation, enabled state, bounded execution lease and fencing counter |
| `eve_sync_runs` | Job/attempt, timings, result, safe error class, HTTP status and counts |
| `eve_esi_cache` | Context-aware request key, validators, expiry, bounded body; encrypted private responses |
| `eve_esi_limits` | Shared group/identity budgets, deferral and atomic coordination |
| `eve_character_profiles` | eve-owned public profile and observation timestamps |
| Extended `eve_credentials` | Existing encryption plus grant generation and refresh revision |
| Existing/extended `eve_role_snapshots` | Compatible access data, grant generation and source validation times |

River owns queue state; business tables own freshness, context and audit. Do not build a second retry engine. Targets must survive cleanup of old River job records.

Separate execution (`idle`, `queued`, `running`, `deferred`, `failed`, `blocked`) from freshness (`never`, `fresh`, `stale`). Blocking reasons include reauthorization, missing scope/role, disabled module and changed identity. Record attempts, full successful checks, content version time and next due time independently. A 304 can be a successful check; it does not invent new content. Avoid misleading aggregate success or fabricated percentages.

Bound execution with a renewable lease and fencing counter. Commit only if both target fencing and grant generation match. Cancel work if lease renewal fails; stale workers cannot overwrite newer results.

## 5. Credentials and authorization consistency

Increment grant generation on context changes or replacement authorization; ordinary token rotation changes only refresh revision. Check current binding and generation before execution and commit.

Coordinate refresh with a character credential row lock. Re-read after locking, refresh only if necessary and commit encrypted credentials immediately. The lock covers a bounded SSO call, not all subsequent ESI requests. Keep ESI requests and refresh tokens inside eve; business consumers do not receive refresh tokens.

SSO rotation and database commit cannot be one distributed transaction. An uncertain rotation/persistence failure requires controlled recovery, with reauthorization if the remaining credential is confirmed invalid. Do not promise lossless recovery in every crash window.

For 401, reload current credentials and allow at most one justified refresh/retry. Classify 403 by endpoint instead of revoking all credentials. Invalidate old corporation facts immediately when a move is observed; wait for relevant old cache periods before publishing newly verified roles. Access accepts only valid facts; pages may show old data with explicit stale state. Never extend all facts merely because a job ran or one request returned 304.

Unlinking stops scheduling, invalidates generations and removes usable credentials/private caches. In-flight results cannot restore unlinked data. Retain only necessary redacted diagnostics.

## 6. Requests, caching, retries and commits

Centralize environment, compatibility date, application identity, timeouts, response size and pagination metadata. Private cache keys include character/generation/scope context; all keys distinguish normalized requests, language and compatibility date. Enforce per-entry and total limits plus cleanup.

Respect cache expiry and validators; no bypass-cache action. A 304 without its matching cached body requires a controlled unconditional retrieval, never an empty successful result.

Atomically coordinate limits by upstream group/identity and legacy egress scope, accounting for in-flight requests. Late responses must not incorrectly increase usable budget. Use conservative endpoint/identity deferral for unclassified 429 and egress-wide deferral for 420. Identity or IP changes must not bypass upstream limits. [CCP limits](https://developers.eveonline.com/docs/services/esi/rate-limiting/)

Proposed transient-error policy: five consecutive attempts with bounded exponential backoff and jitter; then failed state with hourly recovery probes or controlled manual retry. Rate-limit deferral does not consume business-failure attempts. Authorization blocks need an appropriate change; deterministic schema errors stop automatic retries. Validate and configure these initial parameters during implementation.

Commit complete business data and success state transactionally. Repeated execution after an already committed run must not duplicate effects. Partial pagination cannot erase the prior snapshot. Future large snapshots use staging/version publication; history uses natural-key idempotency. [Caching and pagination guidance](https://developers.eveonline.com/docs/services/esi/best-practices/)

## 7. UI and proposed API

Retain [Corporate Clean rules](../ui-design-rules.md). ui-ux-pro-max was consulted for asynchronous feedback and duplicate-submission prevention; it does not replace the project's visual choices.

In My Characters, show two compact resource rows within the current character details, with status, last success and accessible refresh controls. Show a short actionable reauthorization reason when necessary. Do not equate queued with synchronized.

Proposed `/sync` administration: title, search/status filters and a list with avatar/name, dataset, status, last success, next run and action. Expand diagnostics only when needed. Reflow to compact mobile rows with visible state/actions; no charts, countdowns or infrastructure filler. Poll active status about every five seconds, slow down when idle and pause when hidden. Preserve focus and row placement. Test desktop, 375px, 320px, 200% zoom and keyboard use.

All routes below are proposals:

| Route | Access |
| --- | --- |
| `GET /api/v1/eve/sync/characters/{id}` | Session, `eve.sync.self`, current ownership |
| `POST /api/v1/eve/sync/characters/{id}/refresh` | Same plus CSRF; resource allowlist only |
| `GET /api/v1/eve/sync/targets` | `eve.sync.manage`; paginated operational metadata |
| `GET /api/v1/eve/sync/targets/{id}/runs` | `eve.sync.manage`; redacted run details |
| `POST /api/v1/eve/sync/targets/{id}/retry` | `eve.sync.manage`, CSRF, audit and upstream constraints |

`eve.sync.self` is an ownership-constrained member capability, not a grant of site-wide access. `eve.sync.manage` is a separate global operational capability, added to the configurable catalog only when its feature ships. It does not grant private datasets and is not implicitly bundled with `access.manage`; existing superadmin policy still applies.

Accepted requests return 202 with queued/already_pending/deferred outcome, target and eligible time. Authorization blocks return an explicit actionable error. Use consistent rejection for foreign/nonexistent characters, string IDs and UTC ISO 8601 times. Update OpenAPI during implementation.

## 8. Delivery, migration and operation

| Stage | Exit criteria |
| --- | --- |
| A: queue foundation | Pinned compatible River version, migrations, registration/lifecycle, targets/runs, transactional dispatch; real PostgreSQL recovery tests |
| B: client and credentials | Rotation coordination, generations, cache, shared limits and failure tests |
| C: dataset migration | Two initial workers, compatible access DTO, target backfill and exclusive cutover |
| D: UI and operations | Self-service/admin APIs, permissions, audit and responsive verification |
| E: integration | Live authorized-character checks, revocation/move boundaries and bilingual operating docs |

Cutover: stop/drain old worker → backup and additive migrations → backfill existing targets/due times → start new worker → verify snapshots and progress. Preserve successful data and never run both schedulers on the same role dataset.

Rollback stops new workers first and uses a verified compatible old mode. Keep additive tables; avoid destructive downs. Retain legacy scheduling fields for a transition release. Credential-state changes mean an arbitrary old binary is not automatically safe; prefer forward repair if rollback compatibility is unavailable.

Validate modules, migrations and registration before dispatch. Stop scheduling and administrative submissions before draining/cancelling workers; close database pools last. Disabled modules retain explainable pending work, resume compatible resource versions and handle backlog before removing old workers.

Initial retention proposal: successful run details 14 days, failed diagnostics 30 days, manual-operation audit governed separately. Cleanup is bounded and preserves last-success metadata. Observe queue wait, freshness lag, repeated failures, reauthorization and limit deferral.

## 9. Acceptance and current boundary

Verify duplicate/transactional dispatch, concurrent scanners, crash recovery, stale-worker fencing, replay after commit, token rotation, replacement authorization, unlinking, scope/role failures, corporation changes, cache isolation, 304, pagination failure, 420/429, shared budgets and missing headers.

Verify self/admin object restrictions and redaction; regress login, binding, main-character selection, community profile and permissions. Validate River and Goose migrations separately. Record live EVE and production performance checks separately from mocked tests.

This plan installs no dependency and creates no database table, permission, API or page. Implementation stages must update status, architecture, development instructions, OpenAPI, changelog and both language editions.
