# Corporation attendance and estimated online time

> Status index (2026-09-23): this business capability is deployed; remaining extensions and live verification gaps are tracked in the [backlog](../backlog.md). See [current project status](../project-status.md) for versions. Dated/Goose introduction notes describe their historical stage, not today’s release state. Apply current repository migrations, not only the initial module version.

## Corporation PAP and alliance PAP (Goose 44 policy, Goose 45 snapshots)

The two sources are stored and displayed separately. Activity roll calls and organizer-issued awards produce **corporation PAP**, with no monthly threshold and separately configured coin conversion. The WinterCo current-month snapshot is **alliance PAP**, summed per natural person with a default requirement of 3 PAP per person per month. It is viewed in the separate Attendance → Alliance PAP subpage. The River attendance job still reads a complete current-month snapshot every 30 minutes because the upstream service has no delta endpoint; the server transaction updates only changed rows and prunes characters that disappeared from a successful complete snapshot. A failed or incomplete read preserves the previous snapshot and is never treated as zero. Alliance PAP does not automatically issue local PAP or coins; an administrator can manually convert it from the Alliance PAP subpage each month using the independent `alliance_pap` rate, with the existing idempotency and coin-ledger audit.

On the same Attendance → PAP → Corporation PAP page, a manager can open **Issue all** to process closed activities that have bound present participants and have not received PAP. The batch still calls the existing per-event issuance endpoint sequentially: every activity keeps its own version, roster, idempotency key, coin ledger and audit trail. A failed activity does not roll back earlier successful activities; the dialog reports each result and the list can be refreshed for a retry. The per-activity issue, correction and revoke controls remain available in the activity detail, while members do not see the batch entry.

The alliance requirement uses `GET/POST /api/v1/attendance/pap-requirement` and always returns `source: "alliance"`. `GET /api/v1/attendance/alliance-pap` returns the current account total, sync state, last sync time, snapshot version and character rows mapped to that site account; unbound rows are not exposed. Administrators can use `GET /api/v1/attendance/alliance-pap/conversions` to list retained complete months with unconverted balances, then use `GET/POST /api/v1/attendance/alliance-pap/conversion?month=YYYY-MM` to preview and settle a selected month; omitting the month keeps the current-month compatibility path. Alliance and corporation rates are configured independently in exchange, and converted 0.01-PAP units are never issued twice. It is configured under Attendance → PAP and writable only by current site administrators. POST keeps the existing attendance.self guard, host CSRF, optimistic version and audit. The range is 1–100000 integer points, default 3. This policy only affects alliance PAP compliance display; it does not issue/revoke corporation PAP or directly block welfare.

The administrator-only `GET /api/v1/attendance/alliance-pap/summary` endpoint powers the operations dashboard's alliance fulfillment rate. It counts one eligible member per bound site account, sums all bound characters for that account, and compares the result with the configured monthly target. The response reports achieved accounts, eligible accounts and an integer `rate_bps`; incomplete or failed snapshots return `available=false` rather than treating missing rows as failures.

Administrators can also call `GET /api/v1/attendance/alliance-pap/member-months` to list selectable months, then call `GET /api/v1/attendance/alliance-pap/members?month=YYYY-MM` to view member points for a selected month. The endpoint groups multiple characters by their currently bound site account and returns the member name, total PAP, target status and character rows. It rechecks live bindings before returning data and omits rows whose binding is stale, unbound or assigned elsewhere. Omitting `month` selects the current month. If a local snapshot remains after an upstream failure, administrators can still read its member rows and the response retains `state=error` so the UI can show the sync warning. When the current month has no local snapshot, the administrator page can still select a complete historical month.

Configuration changes never rewrite corporation PAP, mint/deduct coins, block welfare, or apply historical penalties. Until alliance data is synchronized, no compliance result is shown. This is a global policy, not account-owned assets to merge; audit preserves original actors.

Back up the local database/configuration, run npm run db:migrate (Goose 45 plus River), and restart the API. The differential persistence needs no new migration. Roll back frontend/backend together if needed; retain policy/audit tables without Down. The sync uses `WINTERCO_PAP_URL` and a restricted `WINTERCO_PAP_AUTH_FILE`; no new ESI scopes are required. The auth file prefers `read_token`; `api_token` is accepted as a compatibility fallback for existing seat-pap files. The token is read only when making the request and never enters a job payload or log.


2026-09-20, local UI update: PAP issue/correction/revocation, manual coin conversion and activity loss inclusion/exclusion use shared compact dialogs. Existing previews, reasons, errors and idempotency behavior are retained. Pending submissions block dismissal and duplicate actions; closing restores trigger focus. This changes frontend form placement only, with no permission, API, transaction, synchronization or migration changes. See the [attendance UI notes](../ui/attendance.md).

2026-09-19 bilingual data update: business requests send `Accept-Language: zh-CN|en`; the server selects presentation language in request context and returns `Content-Language` and `Vary: Accept-Language`, retaining private-response `no-store`. Local SDE type/system names prefer the requested language, with the other language, existing type-name cache or ID as fallback. Generated contract summaries and registered server messages follow the locale. Player names/notes/descriptions, business IDs, permissions, prices, content tokens and ESI caches remain unchanged. No new migration, scope or SDE import; local only. See [interface language](../ui/language.en.md).

## Corporation reimbursement link

Corporation reimbursement (`srp`) now requires a confirmed attendance loss for the same corporation, account, character and killmail, with the attendance entry marked present. Candidate/rejected losses, absent entries, another account’s history and a typed event ID do not qualify. The event need not be closed or have PAP awarded. PVP reimbursement (`solo`) remains available without attendance; new alliance applications remain paused.

An organizer must confirm the candidate in the activity loss list. The host injects public read and transactional-lock services, using the historical attendance account. Rebinding does not transfer attendance; explicit account merges retain their existing ownership migration. No new ESI requests or migration are required.

Goose 30 [account merge](account-merge.en.md) is a separately verified same-member operation: attendance, PAP and battle-task attribution moves to the target while `original_account_id`, original actors and audits retain provenance. Full account merging preserves online samples; individual unlinking/reassignment remains distinct.

Unified Guoke coin redemption is a separate module. PAP-to-coin conversion supports manual and automatic modes (Goose 26); see the [exchange guide](exchange.en.md).

Enabled in production together with exchange on 2026-09-15 at 18:06 UTC+8. The existing Goose 29 schema required no additional migration. All seven characters completed their first online sampling jobs; duration accumulates from enablement. Real fleet capture, fitting/loss evidence and reward delivery still await live activity acceptance; sampling success is not attendance confirmation. See [project status](../project-status.md).

## PAP participation points (Goose 24)

Confirmed: alts accumulate. Each present character earns the event value, summed into the original site-account attribution stored on the attendance entry. One event represents one muster; each character has one current balance per event. Repeated captures never award additional points. The organizer sets an integer value from 1 to 10,000 at issuance, default 1. No online rewards or loss penalties are added; redemption is handled by exchange.

End the event, open the PAP form, enter a value and reason, review the character count and total, then issue. Only present entries with original account attribution qualify. New attendance still requires bound members of the event corporation; legacy unbound entries cannot earn points. Migration never backfills scores automatically; organizers may explicitly issue for eligible historical rosters.

When an activity already has PAP issued and an administrator adds a new eligible attendance record after the activity ends, the detail view shows “Supplement PAP”. The supplement uses the activity's existing per-character score and awards only newly present characters without an award for this activity. Existing characters receive no duplicate score or coin conversion. The supplement writes the PAP ledger, coin entitlement and audit in one transaction and uses its own idempotency key, so retries do not duplicate issuance.

Corrections record the difference to the target balance; revocation sets every event balance to zero while preserving the ledger. A reason, event version and UUID idempotency key are required. Same-key/same-body retries succeed without duplication; mismatched bodies, stale versions and duplicate equal-value issuance fail. Revoke before reopening an awarded event; correct attendance, close and issue again. The event row lock serializes issuance with capture/close/reopen. Balances, nonzero ledger adjustments, version and audit commit atomically, with no ESI requests.

Unlinking or rebinding never transfers historical scores. Members read their original attribution only. The existing `corporation.attendance` capability grants scoped corporation reads and management; administrators use the existing object policy. Session, profile and CSRF guards remain enforced. The manageable capability label includes PAP; its key is unchanged.

Routes: `GET /api/v1/attendance/pap` (own account by default, optional `corporation_id`, `period=month|30d`, zero-based `page`, 50 rows); managers use `GET /api/v1/attendance/pap/pending` to list up to 200 closed activities with bound present participants awaiting issuance; `GET /api/v1/attendance/events/{id}/pap?after=<cursor>` (50 ledger entries); `POST` the event path to issue/correct/revoke. See [OpenAPI](../../api/openapi.yaml). Event and entry DTOs include `pap_points`; events also include `pap_issued`.

Reports use event start dates in UTC+8 for the current month or last 30 calendar days, summing current balances rather than counting adjustment dates. Two alts in a 2-point event earn 4 points, one event and two character participations. Totals cover the entire filter independently of pagination. No leaderboard or arbitrary date range is included; redemption uses a separate coin balance; historical PAP is not backfilled.

Stop the local API and back up database/configuration; run `npm run db:migrate` through Goose 24 and River, then start matching backend/frontend. No new settings, scopes or jobs. Disable attendance before rolling back to code unaware of PAP, keeping all tables and audit history; otherwise old code could bypass the awarded-event reopening guard. Do not run Down for ordinary code rollback.

## System names and exclusion reasons (2026-09-15)

System names require Goose 23 and the mapper-2 SDE import; see the [SDE guide](sde-names.en.md). Entry, ship and loss DTOs add `solar_system_name`, resolved from the active local SDE in Chinese then English. Missing names are empty with a UI ID fallback. Historical system IDs and observation times never change.

Captures still admit only bound members of the event corporation. Results and audits add `excluded_external` and `excluded_unbound`; `excluded` remains their total. Corporation filtering runs first: an external unbound character is counted only as external. Breakdown is saved atomically with the audit and preserved on idempotent replay. Legacy audits return null breakdowns and display an unavailable reason instead of guessing.

2026-09-15. [中文](attendance.zh-CN.md). Implementation: `internal/modules/attendance`; [API contract](../../api/openapi.yaml); current verification/deployment: [project status](../project-status.md). The superseded requirements and unexecuted SQL drafts were removed; see [cleanup notes](../history/README.md).

## Event attendance

Goose 22: new captures skip unbound and external-corporation characters; new manual entries require a site binding. Legacy unbound history stays intact. The earlier unbound character count below only applies to legacy entries.

An organizer creates an event and captures the current fleet with **their own actively bound character**. Only bound characters whose ESI affiliation matches the event corporation are newly recorded; excluded counts include a separate external/unbound breakdown. Affiliation follows ESI cache freshness, not instantaneous membership events. Capture requires the event to have started and remain open. While an event is open, only a live fleet roll call is allowed; manual additions and edits are rejected. After closing, an administrator can add or revise entries, but ESI cannot retrieve a historical fleet roster. If the source fleet endpoint explicitly reports that the character is no longer in a fleet, the event closes automatically; network, rate-limit, and authorization failures never count as a disband. Reopening still requires a reason and returns the event to the open state.

Keep character entries; count distinct attributed site accounts as people. Legacy unbound characters count only as characters; no new unbound participation is created. Online presence never implies event participation, absence, points, or penalties. New manual entries require a site binding and verified corporation affiliation; existing entries remain correctable after a character leaves the corporation. Manual corrections require a reason, survive subsequent fleet captures, and are not automatically cancelled when a later roster omits a character.

Writes use an event version and UUID `request_key`. Identical retries are idempotent; changed content with the same key, stale versions, and invalid state transitions return 409. Capture/manual/close/reopen commit their data and audit together; event creation stores the creator and creation key. The account attributed when a character is first recorded stays immutable through unlinking, rebinding, main-character changes, or later manual changes. Unattributed entries are not automatically claimed. Event/audit history has no automatic expiry and survives character unlinking.

## Online estimation

ESI provides a current observation, **not a complete session history**. Collection starts on enablement; do not backfill earlier duration from `last_login` or interpret lifetime `logins` as daily logins.

- The EVE River runtime samples actively bound characters through existing token refresh, shared caching/rate limits, grant generations, leases, and fencing. Missing scopes block the target; a new authorization generation lets dispatch seed it again.
- The official client cache TTL for online is 60 seconds. Scheduling follows actual Cache-Control/Expires, with a minimum 60-second interval, the existing 1–20-second jitter, and a 30-second dispatch scan. Backlog/rate limits can delay sampling further.
- Successful network validation, including 304, produces an observation. Local cache hits do not fabricate new samples. `ValidatedAt` is separate from `ContentUpdatedAt`; unchanged content can be freshly validated. Cached responses expose the existing validation time with `Cached=true`.
- Estimate an interval only when adjacent samples both say online, share a grant generation, and are at most 300 seconds apart. Failed requests insert a null break. Offline, missing samples, long gaps, and grant changes break continuity. Never extrapolate the last online sample to now. Existing observations are recalculated on read; their stored evidence is unchanged.
- Union overlapping character intervals within each site account. Split by UTC+08:00 calendar days; corporation totals sum account hours, explicitly labelled person-hours. The first scoped active character by ID labels an account, and is not represented as its main character.
- UI supports 7 or 30 days. Raw observations retain 31 days for calendar/boundary handling, with up to 10,000 expired rows removed per maintenance pass. There are no long-term rollups, monthly archives, or retention settings yet.

Days with no successful samples display `—`; sampled days with no estimable online interval can display zero. A failed, expired (>300 seconds), superseded, or unauthorized latest observation is unknown/needs authorization, never silently offline. Sample counts are available in the daily table and do not promise full-day coverage. Each bounded dispatch scan prioritizes online targets and allows a larger batch so background resource backlog does not repeatedly postpone online sampling; official cache headers and shared rate limits still apply.

Each sample stores the corporation from a valid authorization snapshot at publication, or 0 when no such snapshot exists. Corporation reports require both currently scoped active bindings and matching sample-time corporation at both interval endpoints; a new corporation cannot read pre-join online history. Corporation facts themselves have ESI cache delay. Unlinking deletes online samples with the credential; detected owner-hash changes clear prior samples. Reauthorization preserves data but never joins intervals across generations. These lifecycle rules deliberately differ from immutable event-account attribution.

## Permissions and module boundaries

`attendance` depends on `identity`, `eve`, and `access`. Host-protected `attendance.self` routes require a session, profile completion, and CSRF on writes; handlers enforce object scope separately. The newly delivered `corporation.attendance` permission supports corporation/alliance filters. Current site administrators, fresh CEO/Director facts, or explicit delegated grants manage events and corporation activity. Site permissions do not grant game permissions or let organizers borrow someone else's fleet token.

Members read their own historical event entries and account activity. Administrators can read a member's currently active bound characters from the member page; the current administrator flag is checked per request. Historical event entries stay readable by their originally attributed account after unlinking, without expanding access to the unlinked character's contracts or online data.

The host injects `Fleet`, `ActivityProfiles`, `GuardFleet`, and `OnlineDataTx`; credentials remain inside EVE. Identity supplies batch binding lookup and transaction locks. Attendance owns events/entries/audit SQL; EVE owns samples and aggregation queries. Network work stays outside publication transactions. Recheck management authority after fetching, then lock sorted characters → source credential → event, validating ownership, generation, and event version. Transactional database reads use the existing transaction, without waiting for a second pool connection.

## Official endpoints

Checked against [CCP OpenAPI, compatibility date 2026-08-18](https://esi.evetech.net/meta/openapi.json?compatibility_date=2026-08-18) on 2026-09-15:

| Endpoint | Scope | Client TTL | Official group |
| --- | --- | --- | --- |
| GET /characters/{character_id}/online | esi-location.read_online.v1 | 60s | char-location, 1200/15m |
| GET /characters/{character_id}/fleet | esi-fleets.read_fleet.v1 | 60s | fleet, 1800/15m |
| GET /fleets/{fleet_id}/members | esi-fleets.read_fleet.v1 | 5s | fleet, 1800/15m |
| POST /characters/affiliation | Public | Response headers | Official catalogue/actual response |
| POST /universe/names | Public | Response headers | Official catalogue/actual response |

Both scopes already belong to the 57-scope login profile; individual characters may still need reauthorization. ESI decides fleet visibility: denied access, no fleet, and failed requests do not create attendance. Goose 22 stores hull, system and join time per observation. SDE cannot supply character names or online history.

## Enablement, upgrade, and rollback

Back up database/configuration and stop the old service. `npm run db:migrate` maintains both Goose and River lines; the current application schema is Goose **29**, including the attendance, battle evidence, system names, PAP and exchange migrations 21–26, never draft SQL. Enabling these modules on existing production Goose 29 requires no additional migration. Append `attendance` to the existing `MODULES` list, preserving other modules; default is `system,identity,eve,access,community,attendance`. Deploy matching frontend/backend and restart. Dispatch creates online targets for existing grants without inventing historical data. Missing scopes block explicitly; a new grant is recognized by the next dispatch.

Use `/attendance` for events, `/attendance?view=online` for charts, and the online resource in `/sync` for task/error/next-run observation. Disabling attendance in this version stops new online dispatch; existing online handlers snooze for an hour pending reenablement. Tables remain and other resources continue.

**Rolling back to an older binary unaware of online** requires stopping the service, cancelling nonterminal `eve.character-online.v1` River jobs, and marking online targets `blocked` with reason `module_disabled`, cleared active job/lease, and incremented fence. Restore the old `MODULES` and release before starting. Retain Goose 21 data; do not run Down for a normal code rollback. Otherwise the old dispatcher encounters unknown resources. Re-upgrading restores `module_disabled` targets. Full database restoration is a separate deliberate data rollback.

Database/HTTP fixtures cover attribution, deduplication, external-corporation filtering, manual precedence, idempotency, concurrent versions, grant fencing, revoked administrators, single-connection pools, sample gaps/failures, and corporation boundaries. Browser fixtures cover workflows, chart/table, unknown versus zero, and 1440/375/320px layouts. Check [project status](../project-status.md) for actual production sampling and fleet verification; screenshots are fixtures, not real attendance.

## Ship, fitting and loss evidence (Goose 22)

Visibility fix: event entries expose `solar_system_id` and `location_observed_at` from the newest fleet observation by observation time, both null when no valid system was recorded. The roster directly shows capture system/time; ship and loss summaries show their respective systems before fitting expansion. Never substitute the pilot's current location. This reads existing data and requires no new migration.

Each capture adds an immutable ship observation with hull, system, join time and roster validation time. Replay does not duplicate it; later ships never replace it. Corporation membership or online presence alone is not participation.

Workers use each participant's own `esi-location.read_ship_type.v1` and `esi-assets.read_assets.v1`. Current ship provides the item ID; all asset pages are read, retaining direct children with that location ID (modules, drones and cargo). Other assets are not copied into attendance. Private ESI caches remain encrypted and grant-isolated. Nested asset containers, module online state and mutated module attributes are not reconstructed.

Ship TTL is 5 seconds and asset TTL 3600 seconds, subject to actual headers. Store ship validation time, oldest asset-page validation time and content time. This is not an atomic fit at roll call. Check ship ID/type before and after assets. A task starting over 3 minutes after capture, a ship change or missing assets produces an unavailable state. Same-type swaps before the first private observation cannot be detected from the roster. Pagination is bounded to 100 pages; changing page counts and incomplete reads do not publish. Network budget is 55 seconds; temporary retries reuse cache, stopping after five failures. Capture again for a fresh fitting observation.

Losses use personal `/characters/{character_id}/killmails/recent` and public `/killmails/{killmail_id}/{killmail_hash}`. Keep only the participant as victim within the event window, not every kill in the list. Personal authorization covers own losses without corporation roles. The list covers 90 days; older reports cannot be promised. Closing records an end time, reopening clears it. Scans freeze a cutoff per pass and persist pagination. Complete passes respect list expiry and a minimum 5-minute interval. Closed events continue for 24 hours, then managers may retry manually.

Killmails have no event ID: matches start as candidates. Managers confirm or exclude with reasons, versions and idempotency keys. A killmail can be confirmed for only one event; sync never overwrites reviews. Keep hull, time, system, numeric location flags, nested items and destroyed/dropped quantities. Missing items are not zero loss. No ISK valuation or SRP payment is introduced. Types and systems use shared SDE Chinese-first names; missing names and unmapped loss flags retain IDs.

`attendance.battle.v1` and `attendance.dispatch.v1` join the shared River runtime with two attendance workers and a 30-second dispatcher. Tasks are saved in the business transaction; job payloads contain only task IDs. Publication checks a 90-second lease/fence, original account, owner hash and grant generation, locking identity → credential → event → task. Network stays outside transactions. Unlinking/rebinding stops collection into old attribution; saved evidence remains historical. Members read only original entries; managers read scoped events. This does not grant unrestricted asset browsing.

GET only reads local evidence: last 20 ship observations and 100 losses, with a truncation flag and no deletion. Missing scopes, failed and unchecked states stay distinct. After reauthorization, managers can retry losses without bypassing cache/budgets. Under `/api/v1/attendance`: GET `/events/{id}/characters/{character_id}/battle`; POST that path plus `/refresh`; POST `/events/{id}/losses/{loss_id}/review`. Writes require `corporation.attendance`, CSRF and object checks.

Upgrade: stop the local service, back up database/key, run `npm run db:migrate` through Goose 22 and River, then start matching frontend/backend. Disabling attendance snoozes workers and preserves data. Before rollback to an older binary, also cancel nonterminal `attendance.battle.v1` and `attendance.dispatch.v1` jobs and retain new tables. Older code permits unbound roll call and is not behavior-compatible.

Sources: [official OpenAPI](https://esi.evetech.net/meta/openapi.json?compatibility_date=2026-08-18), [official asset relationships](https://docs.esi.evetech.net/docs/asset_location_id.html). Real fleet/asset/loss integration remains to be verified with actual participants; fixtures do not establish live integration.
