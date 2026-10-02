# Skill management

> Status index (2026-09-23): this business capability is deployed; remaining extensions and live verification gaps are tracked in the [backlog](../backlog.md). See [current project status](../project-status.md) for versions. Dated/Goose introduction notes describe their historical stage, not today’s release state. Apply current repository migrations, not only the initial module version.

2026-09-19 bilingual data update: business requests send `Accept-Language: zh-CN|en`; the server selects presentation language in request context and returns `Content-Language` and `Vary: Accept-Language`, retaining private-response `no-store`. Local SDE type/system names prefer the requested language, with the other language, existing type-name cache or ID as fallback. Generated contract summaries and registered server messages follow the locale. Player names/notes/descriptions, business IDs, permissions, prices, content tokens and ESI caches remain unchanged. No new migration, scope or SDE import; local only. See [interface language](../ui/language.en.md).

## Remaining skill points (local, 2026-09-17)

Plan checks now return nullable non-negative integer `remaining_sp` per character and requirement. The character total sums the listed requirements only, without expanding additional prerequisites. A met requirement has zero deficit. Missing levels use `ceil(250 * SDE skillTimeConstant * 2^(2.5 * (required_level - 1)))` minus observed skill points, with the confirmed trained level as a minimum. Apply the rank before rounding; observed partial-level progress counts. Unallocated SP is not deducted and active training is not extrapolated from the clock.

Existing observation rules for completed queues remain: a confirmed level supplies its minimum points if end SP is absent. Missing authorization/data/rank, conflicting levels or inconsistent target SP produce null. Any unknown component makes the total null while known components remain visible. Authorized historical observations remain usable until replaced by synchronization. No permission change, database migration or additional ESI request is introduced.

Ranks for all 511 catalog skills are extracted by `scripts/skill-catalog.py` from the same official SDE build's `typeDogma.jsonl`. Formula: [EVE developer documentation](https://developers.eveonline.com/docs/guides/useful-formulae/#skillpoints-needed-per-level). Whole-point rounding was cross-checked against real local ESI queue `level_end_sp`: skill 20494 level IV = 90,510 SP; skill 16069 level II = 2,829 SP. Automatic name updates do not replace this compiled skill catalog.

Skill management shipped on 2026-09-15; snapshot retention and historical checks shipped on 2026-09-16 with v0.1.0-account-merge-fixes-20260916.

## Scope

`/skills` provides trained skills, the training queue, corporation requirement plans, and per-character checks. Users can inspect their active bound characters; current site administrators can open another account through `/skills?member=<account UUID>` from the member directory. Alts are checked separately; their levels are never added together.

Corporation plans contain a name and 1–200 skill/level requirements (levels 1–5), with up to 100 plans per corporation. Create retries use a UUID request key; updates/deletes use optimistic versions and transactional audit records. `corporation.skills` allows plan maintenance and requirement-only checks for bound members of that corporation, not access to their entire skill sheets or queues. Ordinary members see their corporation's plans and their own results.

No game queue modification, skill-point allocation, skill injection/purchase, personal training-plan optimization, or in-game plan writeback is included.

## ESI integration

Fix, 2026-09-16: normal reauthorization for the same character and owner hash, with continuously granted resource scopes, carries the immediately preceding grant's skills/queue observations into the current generation inside the credential transaction. Payloads and `observed_at` stay unchanged; target `valid_until` is cleared. The UI keeps existing data without an update-pending label, replacing it after successful synchronization. The API still reports `stale`; corporation checks can also use these historical observations without treating them as newly observed data.

Changed ownership, removed scopes, revoked credentials, unlinking, or older generations cannot restore prior data. Personal fitting snapshots are excluded. Retention is independent of module UI switches; reads still require a valid binding and current permissions. Old jobs keep their old generation/fence and cannot publish. This fix needs no separate migration and is deployed; account merge in the same release upgrades the production database to Goose 30.

Verified against the [official Tranquility OpenAPI](https://esi.evetech.net/meta/openapi.json?compatibility_date=2026-09-14) on 2026-09-15; it returned version `2026-08-18`. See the [ESI overview](https://developers.eveonline.com/docs/services/esi/overview/).

| ESI route | Required scope |
| --- | --- |
| `GET /characters/{character_id}/skills` | `esi-skills.read_skills.v1` |
| `GET /characters/{character_id}/skillqueue` | `esi-skills.read_skillqueue.v1` |

Both scopes already belong to the existing login baseline. Missing scopes require the character owner to reauthorize; site permissions never replace CCP authorization. The current official metadata reports event-based caching, a 60-second client TTL, and the `char-detail` rate group (600 tokens / 15m). Actual scheduling follows shared-client response headers, Expires, cache revalidation and shared rate limits. River checks due targets approximately every 30 seconds; backoff and queue load may delay work. The UI reads local snapshots every 60 seconds; its refresh button does not bypass the ESI cache.

Either `skills` or `fittings` enables the one shared skill target per character. Only `skills` enables `skillqueue`. All work retains authorization generations, binding checks, leases/fences, and transactional publication. Jobs carry identifiers, never tokens.

## Reconciliation and freshness

The official documentation explains that offline completed training can remain absent from `/skills` until the character logs into the game. Display calculations retain raw snapshots and overlay completed queue levels/end SP when confirmed by the queue observation. They do not promote active levels: Alpha restrictions and expert systems can make active and trained levels differ.

- Completion must precede the queue observation, not merely the browser clock. A queue observed before the skill snapshot does not overwrite the newer skill observation; conflicting levels yield unknown checks for the affected requirements.
- Corporation requirements use trained levels from the latest readable skills and queue observations. Both `ready` and `stale` are usable; expiry schedules refresh without clearing results. Checks reread local data every 60 seconds and recompute when replacement observations become available, preserving the observation timestamp.
- Pending/blocked resources, missing payloads or observation times, and conflicting observations remain unknown rather than a zero-level failure. Historical results do not claim real-time game state; future training is never marked complete solely because wall-clock time has advanced.
- Missing optional unallocated SP remains unknown. Queue progress is a date-based estimate; entries without start/end times show paused or unscheduled. A passed expected finish time without a confirming observation remains awaiting confirmation.

## Storage and API

Goose 28 adds `skillqueue` to the EVE-private `eve_fitting_snapshots` resource constraint; its historical name remains compatible with Goose 27. The skills module owns `skills_plans` and `skills_plan_audit` (before/after values for create/update/delete). No network request runs in a plan-write transaction. Host-injected services provide binding checks, current administrator flags, corporation RBAC, snapshots and StaticDataService; no cross-module store imports are used.

All routes require the session-protected `skills.self` capability and additional object checks. Corporation membership/management relies on valid current authorization facts; expired facts do not grant corporation access.

| Site route | Purpose |
| --- | --- |
| `GET /api/v1/skills/context?member=` | Subject's active characters and actor's visible corporations |
| `GET /api/v1/skills/catalog` | Skill picker catalog and build |
| `GET /api/v1/skills/characters/{id}` | Owner/current administrator: complete skills, queue and freshness |
| `GET /api/v1/skills/plans?corporation_id=` | Visible corporation plans |
| `POST /api/v1/skills/plans` | Create with UUID `request_key` |
| `PUT /api/v1/skills/plans/{id}` | Version-checked update; corporation cannot change |
| `DELETE /api/v1/skills/plans/{id}` | Version-checked deletion |
| `GET /api/v1/skills/plans/{id}/check?member=&after=` | Owner/admin subject checks, limited to the plan's corporation |
| `GET /api/v1/skills/plans/{id}/check?scope=corporation&after=` | Authorized corporation manager checks; 50 characters/page |

Check responses contain `items` and `next_cursor`. Member and corporation scopes cannot be combined. IDs, versions and cursors are decimal strings. Writes require Origin and `X-CSRF-Token`; errors use 400 invalid input, 404 unavailable/forbidden object, 409 version/retry conflict or plan limit, and 503 service unavailable. See [OpenAPI](../../api/openapi.yaml).

## Static data and upgrade

Names prefer current local SDE Chinese/English through StaticDataService. Type validation and group labels use 511 published category-16 skills from official SDE build `3503375`, with pinned names as fallback. This small compiled catalog is independent of the frontend Dogma package. Automatic database name updates do not update this compiled category allowlist.

To rebuild from a verified official JSONL archive: `python scripts/skill-catalog.py <official-sde-jsonl.zip> --build <build>`. Update the catalog build and documentation and rebuild the backend. No per-type ESI requests are made by this page.

Back up the database and `.env`, run `npm run db:migrate` (Goose and River), append `skills` to the existing `MODULES` list, run `npm run build`, and restart. Goose 28 was validated locally and deployed with the 2026-09-15 fitting-library release (production Goose 29); skills and queue snapshots synchronized for seven characters. Down migration destroys plans, audits and queue snapshots; it is not a lossless rollback.

## Plans from fittings

The corporation fitting library can seed the shared plan editor with recursive minimum SDE prerequisites. Change levels, add/remove skills, then save through existing skills permission/version/audit endpoints. Immediate re-edit and `/skills?plan=<id>&corporation=<id>` navigation are supported; URL parameters never bypass authorization. See [fittings](fittings.en.md).

Sync management/character views now validate skillqueue and allow its manual refresh, avoiding rejection of the entire sync list. See [sync guide](esi-sync.en.md).
