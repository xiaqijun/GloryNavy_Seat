# SeAT ESI synchronization research and project recommendations

> Baseline / historical research: recommendations here do not imply delivery. See the [module guides](README.md), [current status](../project-status.md) and [backlog](../backlog.md). Original upstream verification dates are retained.

Verified: 2026-09-14. Status: **research and recommendations; no general sync framework implemented**. [中文](seat-esi-sync.zh-CN.md)

## 1. Conclusion and scope

SeAT separates scheduling, entity batches, resource jobs, an ESI client and persisted models. Adopt these boundaries and staggered execution in the existing Go modular monolith, following the project's planned River + PostgreSQL direction.

Source review uses `eveseat/eveapi` commit `990a0a29649d0701f35605739c2e9b823275a13a` dated 2026-08-09. The latest-release API returned 5.0.37; the reviewed branch snapshot is not asserted to be that release tag or every deployed installation. No SeAT runtime, live ESI synchronization or throughput test was performed.

## 2. SeAT processing flow

Scheduler/manual update → eligible entities → Character/Corporation batches → Redis queue → Horizon workers → middleware → EsiClient/Eseye → ESI → resource mapping and database writes → local data consumed by pages.

Current commands, jobs and models live in `eveapi`; historical references to the old console package are not the current source entry point. See [dependencies](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/composer.json).

The main ESI queues are `characters`, `corporations` and `public`. A queue named `high` does not automatically have execution priority. [Queue documentation](https://eveseat.github.io/docs/developer_guides/job_queue_flow/)

`EsiClient` is bound to `EseyeClient`. Its separate response-cache store defaults to files and can use Redis; queue storage and response caching are distinct. [Provider](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/EseyeServiceProvider.php), [cache configuration](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Config/eseye-cache.php)

## 3. Scheduling

Bucket sizing uses a 120-second estimated batch duration and a 3600-second update window, producing up to approximately 30 buckets. These are SeAT parameters, not ESI protocol limits. [BucketManager](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Traits/BucketManager.php)

The seeded schedule advances one bucket every two minutes. Per-character scheduling has a one-hour minimum interval and records `last_update` before dispatch, so that timestamp is not proof of completed synchronization. There is a token keep-alive branch for longer intervals. [Update command](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Commands/Seat/Buckets/Update.php)

Other commands cover shorter-cycle datasets, and some schedules receive random offsets. Defaults are not the installation's effective settings. [ScheduleSeeder](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/database/seeders/ScheduleSeeder.php)

Project recommendation: persist due times per resource/entity, combining cache expiry, retry delay and jitter. Manual refresh must obey the same constraints.

## 4. Jobs, authorization and multiple characters

Batch construction filters authenticated jobs by granted scopes. Jobs declare endpoints, compatibility dates and mapping logic. Entity batches wrap a job chain; they should not be described as all resources running concurrently. `CharacterBatchProcessed` is emitted in `finally`, including failed batches. [Bus](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Bus/Bus.php), [Character](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Bus/Character.php), [Corporation](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Bus/Corporation.php)

Automatic bucket dispatch selects Director characters for corporation updates and deduplicates corporations within that command execution. This is not durable deduplication across workers or buckets. Corporation middleware checks scope, token version, NPC status and required roles. The role check reads normal `roles` and accepts Director. [Dispatch](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Commands/Seat/Buckets/Update.php), [role middleware](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Jobs/Middleware/RequireCorporationRole.php)

Project recommendations:

- Keep credentials and sync state independent per linked character; the main character is a display choice.
- Deduplicate corporation work by corporation/resource and track the source character separately. Recheck eligibility before execution; do not switch identities to bypass limits.
- Keep upstream scopes, game roles and website permissions separate.
- Enabled modules explicitly register jobs and own their persistence logic through private stores.

## 5. Credentials, caching, limits and failures

SeAT persists rotated credentials through `InteractsWithToken`, including after ESI request failures. Specific permanently invalid refresh-token errors lead to token removal. Authenticated jobs configure overlap middleware; the character ID alone does not prove cross-class refresh locking. [Token persistence](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/InteractsWithToken.php), [authenticated jobs](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Jobs/AbstractAuthCorporationJob.php)

The reviewed base job permits three unhandled exceptions and increasing backoff. Its internal shared error threshold is 80 with a 300-second delay. Route-status middleware is commented out in the default list, while server-status checking is enabled. These details do not establish full compliance with today's upstream limits. [EsiBase](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Jobs/EsiBase.php), [limiter](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Jobs/Middleware/CheckEsiRateLimit.php)

Current CCP guidance requires handling both legacy error limits and gradually introduced grouped rolling limits. Use response headers and Retry-After; scheduling buckets are unrelated to upstream limit buckets. [CCP limits](https://developers.eveonline.com/docs/services/esi/rate-limiting/)

Respect Expires, use conditional GET where supported, check paginated snapshot consistency and identify the application/contact. These are current integration recommendations, not claims that the reviewed SeAT code implements every detail. [CCP best practices](https://developers.eveonline.com/docs/services/esi/best-practices/)

Project failure handling: retry transient errors with bounded backoff; share relevant rate-limit state across workers; coordinate refresh rotation per credential; require reauthorization for confirmed revocation; block only affected resources for missing scopes/roles or corporation changes. A generic 403 must not automatically revoke the entire credential. Record a 304 as a successful check without inventing a new content version.

## 6. Persistence

SeAT reconciles roles, upserts assets page by page before deleting stale entries after the final page, and deduplicates wallet history by entry identity while walking backward to known data. [Roles](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Jobs/Character/Roles.php), [assets](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Jobs/Assets/Character/Assets.php), [journal](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Jobs/Wallet/Character/Journal.php)

Project recommendation: distinguish replaceable snapshots from append-oriented history. Publish complete validated snapshots; partial page failures must preserve prior data. Use business keys and idempotent consumers for history. Persist attempts, successes, due times, content timestamps, error classes, source characters and batch IDs separately.

SDE remains a separately versioned static-data import. SeAT schedules it independently; its full importer was not audited here. Dynamic character jobs must not repeatedly import SDE.

## 7. Current implementation and proposed next phase

The project already has encrypted per-character credentials and a specialized affiliation/role worker with durable due times and row-lock claiming. Its public cache and pause state are process-local. River is planned, not installed. [Authorization worker](../../internal/modules/eve/authorization.go), [ESI client](../../internal/modules/eve/esi.go), [architecture](../architecture.md)

Introduce the generic queue and migrate existing affiliation, roles and basic corporation information first. Avoid running old and new workers concurrently for the same resource. Separate task claiming, credential-refresh coordination and business commits; the current narrow worker holds a transaction during network calls.

Include resource status and controlled retry operations. Add assets, wallets and their configurable permissions only with the corresponding delivered features. Having 57 granted scopes does not mean every dataset should immediately be fetched.

Acceptance should cover duplicate dispatch, concurrent workers, crash recovery, token rotation, ESI failure after successful refresh, missing authorization, corporation changes, caching/304, rate-limit deferral, partial pagination and redaction. This delivery changes documentation only: no new API, queue, migration or UI.
