# Character loss synchronization

2026-09-24 local, not yet deployed: the detail view separates dropped and destroyed quantities and indents nested cargo using the stored slot path. The path records slot nesting; it does not invent a unique parent for multiple containers in the same slot. `GET /api/v1/welfare/losses/prices?corporation_id=...&character_id=...&killmail_id=...` reuses the loss object's authorization, active binding and credential-owner checks before quoting stored items with the existing Jita 4-4 appraisal service. It returns `items: [{mid, observed_at}]` in the same order as the original items; `mid` is the reference value for the item's full quantity, split for display by dropped/destroyed quantities. Missing quotes are `null`. The quote loads asynchronously when detail opens and does not expose the KM hash, alter the original ESI record or recalculate a frozen reimbursement. It is a current reference, not a price at the time of the killmail. The local page refetches after a six-hour cache window when reopened. No migration or new ESI scope.

2026-09-24 addition: available attacker corporation and alliance names are saved with the report. Detail reads prefer saved names and otherwise use only the local entity-name cache, without blocking review.

2026-09-24 local, not yet deployed: newly synchronized killmails retain ESI `victim.damage_taken`, attackers' damage/final blow/ship/weapon and available names, alongside victim item drops and destruction. Older raw records still in the official recent feed are enriched through the existing cursor. After each completed recent-feed pass, one archived loss can also be refreshed using its already-saved killmail ID/hash; the hash is never exposed to the client and current reports retain priority. Legacy details without attacker data show an explicit pending message. Submitted welfare cases keep their frozen pricing and evidence; authorized detail responses may add display-only fields from the current owner-bound EVE cache without rewriting awards or audit. No source cache is read when binding, scope or game ownership no longer matches. ESI killmails contain no ISK valuation; the site's separate market appraisal must not be presented as a third-party total, drop value or destroyed value. No migration or scope change.

> Status index (2026-09-23): this business capability is deployed; remaining extensions and live verification gaps are tracked in the [backlog](../backlog.md). See [current project status](../project-status.md) for versions. Dated/Goose introduction notes describe their historical stage, not today’s release state. Apply current repository migrations, not only the initial module version.

2026-09-19 bilingual data update: business requests send `Accept-Language: zh-CN|en`; the server selects presentation language in request context and returns `Content-Language` and `Vary: Accept-Language`, retaining private-response `no-store`. Local SDE type/system names prefer the requested language, with the other language, existing type-name cache or ID as fallback. Generated contract summaries and registered server messages follow the locale. Player names/notes/descriptions, business IDs, permissions, prices, content tokens and ESI caches remain unchanged. No new migration, scope or SDE import; local only. See [interface language](../ui/language.en.md).

## Current attendance requirement (2026-09-16)

Corporation reimbursement (`srp`) now requires a confirmed attendance loss for the same corporation, account, character and killmail, with the attendance entry marked present. Candidate/rejected losses, absent entries, another account’s history and a typed event ID do not qualify. The event need not be closed or have PAP awarded. PVP reimbursement (`solo`) remains available without attendance; new alliance applications remain paused.

The list resolves attendance links in a batch; an unlinked loss offers only PVP reimbursement without an additional explanatory attendance warning. Submission and approval lock and recheck the association, including administrator manual entries. The server assigns the event ID; approval requires the same confirmed link. Revocation or absence returns HTTP 409 `welfare_attendance_required`. Legacy pending corporation cases without a valid link cannot be approved: cancel and resubmit after confirming the loss. Already-approved awards are not recalculated. No automatic awards, loss confirmation or policy-configuration gate is introduced.

New applications offer corporation and PVP reimbursement only. `action=apply, kind=alliance` is rejected while alliance work is paused. Existing alliance cases still contribute progress and KM deduplication. The frontend also filters alliance options from older cached responses.

The historical type keys `srp`, `alliance` and `solo` remain readable; current application availability follows the requirements above.

Introduced with Goose 32 on 2026-09-16 and subsequently deployed. [中文](character-losses.zh-CN.md). Open Ship losses from the sidebar at `/losses`. The welfare page no longer opens a loss-browser dialog. Select a report and reimbursement type to open the shared application dialog; submitting it keeps the user on the loss page.

## Official source and lifecycle

Verified against [CCP OpenAPI for compatibility date 2026-08-18](https://esi.evetech.net/meta/openapi.json?compatibility_date=2026-08-18). `GET /characters/{character_id}/killmails/recent/` discovers kills/losses from the past 90 days using the existing `esi-killmails.read_killmails.v1` login scope. Returned ID/hash pairs resolve through `GET /killmails/{killmail_id}/{killmail_hash}/`. Documented TTLs are 300 and 2,592,000 seconds; actual response caching, ETag/304 and shared rate limits govern execution. Manual sync cannot bypass cache.

Welfare enables per-character `killmails` targets and River kind `eve.character-killmails.v1` in the existing `eve_characters` queue. Discovery references and page cursors persist. Each publication processes one new detail; interruptions resume remaining references, and completed references deduplicate by character/game owner/hash. Page-count changes or a later page returning 404 restart discovery. Failed details are not marked processed. Existing backoff, grant-generation and lease/fence checks remain in force.

Only reports where the authorized character is the victim become visible losses. Attacker participation keeps an internal deduplication marker. Records include KM, ship, victim corporation, time, system and dropped/destroyed items with slot paths. Hashes remain private; no ESI valuation is invented. StaticDataService resolves names in batches from SDE Chinese/English and existing name cache/ID fallback.

Pages read local data. Saved losses accumulate and survive refresh/empty discovery. Reauthorization for the same game owner preserves history; removed scope, invalid authorization or changed game ownership blocks old evidence. Unbinding deletes credential-owned raw data, while submitted welfare snapshots remain. Account merge moves the binding without duplicating reports or resetting claims.

## Access and claims

Reimbursement uses human review. Active bound characters may submit PVP claims; corporation claims additionally require the confirmed attendance link above. Policies, effective dates, identity verification, fitting and skill requirements do not gate losses. Historical losses in another victim corporation may be submitted for PVP review. Current account ownership, valid binding and corporation access scope still apply; missing ESI authorization never permits forged synchronized evidence.

The loss page has no qualification or policy controls. Live status reflects availability or the actual case workflow; completed means delivery confirmed. Processing/completed cases hide duplicate application controls. Only future-dated reports are unavailable for date reasons. Status is batch-resolved after raw loss authorization, scoped to the owner's cases or an administrator's authorized character. Submission refreshes local caches.

GET /api/v1/welfare/losses requires corporation_id and character_id; 30 rows, before pagination, killmail_id detail lookup. Items are detail-only. The server reconstructs evidence and rechecks binding, credential, game owner and scope before publication. Missing required synchronized evidence is rejected. Administrator manual entry still targets the operator's own bound character.

Reviewers enter the actual award directly: 0.01–1,000,000,000,000 ISK. No automatic percentage, discipline discount, PVP cap/day limit or mandatory alliance non-payment gate applies. Reviewers check circumstances, alliance outcome, fitting and amount. Unique primary KM compensation, no self-review/delivery, receipts and auditing remain. Growth/capital policy and qualification rules are unchanged.

Stored loss policies remain but are ignored by new applications. Pending cases use manual award review; approved awards and history are not recalculated. Typed KM IDs alone are not verified. No automatic payment, KB valuation or backfill outside the official discovery window is introduced. Attendance matching uses only exact confirmed loss records and does not automatically confirm candidates.

## Release and verification

Back up, run `npm run db:migrate` for Goose 32/River, then start matching builds. No new environment variables. `eve_loss_cursors` and `eve_character_killmails` belong to EVE's private store. Disabling welfare stops new dispatch and snoozes existing workers. Before reverting to an older binary, stop workers, block killmails targets and cancel unfinished jobs of the new kind; do not use Down to erase history.

The first local sweep completed for six targets: 18 reports and 16 victim losses, with no automatic applications/payments. Isolated tests cover resume, cache/deduplication, grant fences, scope/owner isolation, object permissions, server evidence and migration round trips. UI evidence is in the [welfare page notes](../ui/welfare.md).
