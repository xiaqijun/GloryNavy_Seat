# Alliance PAP: standalone service integration

## Historical conversion update 2026-10-02 (deployed, live admin acceptance pending)

Administrators can open the Alliance PAP conversion dialog and select any retained complete month with an unconverted balance. Each month has its own completeness marker and version. Submission rechecks the snapshot version, current valid bindings, the `alliance_pap` rate and already-converted units, then uses the exchange idempotency key and source ledger. Fully converted months are omitted; stale versions require a fresh preview. Historical conversion does not rewrite the snapshot or treat it as the current month.

Production `v0.1.0-alliance-pap-history-20261002` applied Goose 61, backfilling successful-month markers for existing snapshots and recording the marker in the same transaction as future complete publications. A month without a complete publication marker cannot be converted. The administrator-only endpoints are `GET /api/v1/attendance/alliance-pap/conversions`, `GET /api/v1/attendance/alliance-pap/conversion?month=YYYY-MM` and the POST request with `month`; omitting the month remains compatible with the current-month flow. Live administrator preview, ledger verification and settlement for September remain pending.

## Local automatic conversion update 2026-09-23 (deployed)

Administrators can choose manual or automatic for the independent `alliance_pap` rate; upgrading does not change the mode. Each character/month's first observation or first valid binding establishes a baseline. Later successful complete snapshots convert increases only. Administrators may explicitly preview and settle historical baseline points manually. Both paths share references and the same ledger.

PAP uses hundredths; sub-cent coin progress is persisted until it accumulates into a payable unit. Each character/month locks its rate at first actual conversion; corrections use that rate. Decreases reverse converted increases; disappearance from a complete snapshot reverses that month's converted coins. Duplicate, failed, incomplete and wrong-month snapshots issue nothing. Each new month establishes a new baseline without backfilling the prior month.

Publication locks current bindings and active accounts before source version and wallet locks. Stale fetches cannot replace newer publications. Detachment does not transfer historical ownership or grant new coins; explicit account merges migrate snapshots/ledger and include snapshots in the merge preview fingerprint. Snapshot and coin changes commit atomically. The upstream only supplies cumulative monthly totals, not per-event activity records.

Status consolidated 2026-09-23. [中文](winterco-pap.zh-CN.md). Current-month synchronization, server-side delta persistence, reports and administrator-triggered conversion are deployed. **Automatic conversion is deployed; existing manual configuration is unchanged.** See [project status](../project-status.md) and [PAP-01](../backlog.md).

## Production data path

Standalone seat-pap service → private backend HTTP → attendance River queue → local PostgreSQL → Alliance PAP page / workspace. The browser never calls the upstream directly or receives its token/cookies. The local DOM tool is not the production source.

- The standalone service runs from `/root/seat_pap` on port `18770`. It owns upstream sessions and complete current-month snapshots; this application reads authorized APIs only.
- Configure `WINTERCO_PAP_URL` and `WINTERCO_PAP_AUTH_FILE`. A colocated example URL is `http://127.0.0.1:18770`. The reader prefers restricted `read_token`, with compatibility fallback to `api_token`; store the file privately and never commit its contents.
- Goose 45 introduced persisted alliance snapshots. River reads the complete current month every 30 minutes. There is no upstream delta endpoint: network reads remain full snapshots, while the database updates only changed rows and prunes disappeared current-month characters after complete success. Failures, incomplete snapshots and wrong months retain prior data, never zero it.
- The upstream service has its own 30-minute capture clock. The two schedules/caches do not guarantee a game change appears within 30 minutes. Use actual synchronization timestamps.
- Source rows contain decimal monthly cumulative PAP per character, not individual activity IDs/dates. The application supports 0.01 PAP precision and maps valid character bindings to accounts; reports retain object authorization.

## Reports and conversion

`/attendance?view=alliance-pap` shows the current month, default target of 3 PAP per person, a progress ring, mapped character details and synchronization state. Administrators can open “Convert alliance PAP” below the details, select a retained month with an unconverted balance and settle it from that complete snapshot. The workspace reads the same report. Corporation PAP comes from local roll calls, has no monthly threshold, and cannot satisfy the alliance requirement.

- `GET /api/v1/attendance/alliance-pap`: authenticated account report from the database, without upstream reads in page requests.
- `GET/POST /api/v1/attendance/pap-requirement`: read/update the alliance target; mutations require the current site administrator and version/audit checks.
- `GET /api/v1/attendance/alliance-pap/conversions`: administrator list of complete retained months with pending balances and preview summaries.
- `GET/POST /api/v1/attendance/alliance-pap/conversion`: administrator preview/submission for the selected `month=YYYY-MM`; omitting the month keeps the current-month compatibility path. It uses the independent `alliance_pap` rate and idempotent entitlements. Repeated submissions do not mint duplicate coins.
- Alliance snapshots do not become local corporation activity ledger entries. Exchange owns coins; attendance settles via a host-injected transaction service, never another module's store.
- The local `alliance_pap` implementation accepts manual/automatic independently of corporation `pap`. Upgrading preserves existing modes. Automatic mode baselines the first character/month snapshot, then converts complete snapshot deltas and handles downward corrections. Production verification is tracked under PAP-01.

## Authorization and failures

The restricted `read_token` was verified for health, session summaries and current-month snapshots. Historical CSV, cookie updates and session-management operations require the administrative token. Do not expose either credential to the frontend; payloads and logs contain no credentials.

Transient network/5xx failures retry within the task budget; failed runs preserve the previous complete snapshot and error state. Recover expired upstream sessions in the standalone service, never by trusting member-submitted point values. Module disabling and account merges follow [attendance](attendance.en.md) and [exchange](exchange.en.md); do not clear ledgers or repay historical months.

## Evidence and outstanding verification

The first standalone read-only check on 2026-09-21 returned 72 rows with fractional PAP. That was a historical sample, not today's membership count. Application ingestion, reporting and upstream retry fixes were subsequently deployed; see [delivery history](../history/project-status-through-2026-09-23.md).

Continue observing upstream renewal, month boundaries and decreases after conversion. Automatic conversion is deployed with local regression coverage; real increments and month boundaries still need observation after enabling it. Upgrades apply the repository's current Goose and River migrations, rather than stopping at this module's introduction version 45.

## Paused local browser tool

`npm run pap:browser -- --mode login --character <character-ID>` and `--mode inspect` remain for historical investigation, requiring local Playwright and Edge/Chrome. The dedicated `.local/winterco-browser/` profile does not copy personal browser cookies. Users perform initial sign-in and additional verification.

This approach is paused after an upstream login error. Its output is `complete=false`, `importable=false`, `awaiting_adapter` and cannot issue points/coins. Production uses the service API; reviving the DOM tool is not a production prerequisite.
