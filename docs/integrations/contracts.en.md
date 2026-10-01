# Personal and corporation contract synchronization

> Status index (2026-09-23): this business capability is deployed; remaining extensions and live verification gaps are tracked in the [backlog](../backlog.md). See [current project status](../project-status.md) for versions. Dated/Goose introduction notes describe their historical stage, not today’s release state. Apply current repository migrations, not only the initial module version.

2026-09-19 bilingual data update: business requests send `Accept-Language: zh-CN|en`; the server selects presentation language in request context and returns `Content-Language` and `Vary: Accept-Language`, retaining private-response `no-store`. Local SDE type/system names prefer the requested language, with the other language, existing type-name cache or ID as fallback. Generated contract summaries and registered server messages follow the locale. Player names/notes/descriptions, business IDs, permissions, prices, content tokens and ESI caches remain unchanged. No new migration, scope or SDE import; local only. See [interface language](../ui/language.en.md).

Local fix, 2026-09-19: interrupted contract response bodies now back off and retry instead of permanently blocking as response_too_large. Actual oversized bodies are still rejected. Partial responses are neither published nor cached; existing contracts and completed items remain. See [ESI operations](esi-sync.en.md) for recovery after interrupted workers.

2026-09-17: welfare reuses local contract evidence through host-injected services for recommendations, explicit administrator confirmation and finished-status checks. ESI scopes, fetching/cache policy and object access remain unchanged. See [welfare](welfare.en.md).

## Completed-item recovery (2026-09-14)

Reauthorization and corporation source changes retain `ready` for immutable items already fetched successfully, while updating the source and grant generation. Stored items and their original success timestamp survive. Unfetched details still require collection; auction bids remain independently refreshable. Goose 16 repairs legacy pending/failed item rows only when a previous successful fetch exists and no live lease is held. It does not unblock access failures, invent successful timestamps, or mark unfetched data complete; rerunning it is safe.

“Continue pagination” is page-level list progress; the final success marks completion of that list round, not completion of all item/bid jobs. Rate-limited details retry separately and an initial large corporation backlog can span multiple windows. Browser refresh reads current local results. Reauthorizing repeatedly advances the grant generation and replaces jobs; it does not accelerate synchronization.

2026-09-14: synchronization, status presentation and contract viewing implemented. [中文](contracts.zh-CN.md). The [ESI operations guide](esi-sync.en.md) still governs credentials, permissions and migration.

## 1. Data scope

As checked directly against [CCP's current OpenAPI](https://esi.evetech.net/meta/openapi.json) on 2026-09-14, both character and corporation contract `type` enums are `unknown`, `item_exchange`, `auction`, `courier`, and `loan`. Do not invent or infer additional game types. Current game contract categories are item exchange, auction and courier ([official help](https://support.eveonline.com/hc/en-us/sections/201141732-Market-Contracts)). The [official Retribution patch notes](https://www.eveonline.com/news/view/patch-notes-for-retribution-1) confirm that loan contracts were removed from the game; their presence in ESI is compatibility, not evidence they can still be created. The UI groups loan (historical) and unknown under compatibility data, while retaining the official backend enum and original values. `trade_direction` is a separate site-derived display field, not a game contract type; CSV preserves separate columns.

Both character/corporation contract lists, non-courier items and auction bids are supported. Required scopes are `esi-contracts.read_character_contracts.v1` and `esi-contracts.read_corporation_contracts.v1`. Both are already in the 57-scope login configuration; a character without actual consent must reauthorize.

The OpenAPI describes contracts involving the character/corporation as issuer, acceptor or assignee, limited to the previous 30 days plus `in_progress` contracts. This wording must not be taken as proof that corporation responses contain only the corporation's own business. [CCP current OpenAPI](https://esi.evetech.net/meta/openapi.json)

Production observation on 2026-09-14, before filtering: 14,038 of 14,260 corporation-source contracts were assigned to its current alliance. The encrypted response cache for the corporation endpoint's first page contained 1,000 rows, including 996 assigned to that alliance, confirming they came from this endpoint rather than a merge of personal lists. This observation does not establish complete alliance visibility for every corporation or time period. These rows use availability=personal, so availability alone cannot identify business ownership. Goose 17 separates synchronization source from business scope.

### Corporation business scope

The user explicitly excludes unrelated alliance contracts. A corporation contract qualifies if (for_corporation=true AND issuer_corporation_id=the corporation), OR assignee_id=the corporation, OR acceptor_id=the corporation. Contracts issued on behalf of the corporation to its alliance remain eligible. Personal issuance/acceptance by a member or alliance visibility alone is insufficient. Personal-source contracts are unaffected.

Goose 17 adds a stored generated in_scope column and a partial index. Lists filter before SQL pagination; detail/items/bids return 404 for excluded contracts, including for site administrators. Existing corporation object authorization remains mandatory. CSV consumes the same filtered list.

Upstream lists still complete every page and retain base metadata, allowing a previously unrelated contract to become eligible if later accepted by the corporation. Excluded contracts receive no new item/bid jobs; existing details are fenced and marked out_of_scope, and old deliveries retire before token access or ESI requests. Sync counters omit excluded details. Historical metadata/items are not physically deleted. Newly eligible contracts resume detail scheduling and reuse successfully fetched immutable items.

Corporation contracts are not an aggregation of every member's personal contracts. The scopes can overlap, for example when a character issues a contract to a corporation. Personal trades between members do not enter the corporation list merely because of membership. Administrators still use the personal scope to inspect a member's personal contracts.

Records are isolated by `owner_kind + owner_id + contract_id`. Contracts disappearing from the upstream window remain in local history; absence does not imply cancellation or completion. Contract fields use JSONB without Go float64 conversion; bid amounts use PostgreSQL numeric. Items retain quantity, included/requested direction, singleton and raw_quantity blueprint semantics. Entity IDs use integer storage.

### Contract status mapping

Checked directly against the same official OpenAPI on 2026-09-14: both character and corporation contracts use the following ten statuses. Chinese labels are site translations, not CCP-provided enum labels.

| ESI value | Chinese display |
| --- | --- |
| outstanding | 未完成 |
| in_progress | 进行中 |
| finished_issuer | 发起方已完成 |
| finished_contractor | 接受方已完成 |
| finished | 已完成 |
| cancelled | 已取消 |
| rejected | 已拒绝 |
| failed | 已失败 |
| deleted | 已删除 |
| reversed | 已逆转 |

Synchronization, queries and filters retain raw values; list, detail and CSV share one translation map. Outstanding is not universally described as awaiting acceptance, especially for auctions. Reversed is not interpreted as voluntary issuer withdrawal; the official schema does not document its triggers. Issuer-only and contractor-only completion remain distinct with active styling; only exact `finished` receives overall completion styling, never unknown values with a similar prefix. Do not derive an `expired` status from local time or mix item/bid synchronization states into contract status. Status tooltips expose the raw ESI value; unrecognized response values remain visible verbatim.

## 2. Jobs and pagination

Each character receives `character_contracts` and `corporation_contracts` list targets. Their first due time is migration/initial authorization time, independent of old role-sync waits. The existing dispatcher checks every 30 seconds. The new `eve_contracts` queue has concurrency 1, list priority 1 and detail priority 3 (lower runs first), while the character queue retains concurrency 2; requests share database rate limits.

Each list execution handles one page. Page number, total pages, earliest cache expiry and grant generation persist. Page data and detail-job enqueue commit atomically; only the final page marks list success. A changed page count restarts verification from page one. Limits are 10,000 pages, 1,000 contracts per page and the existing 1 MiB response limit; violations fail explicitly.

Items and bids use independent River jobs, state and lease/fence protection. Successfully fetched items are treated as fixed contract contents. Courier package contents are not fetched, and deleted contracts create no new detail jobs. Corporation bids paginate independently and upsert by bid_id. Replay does not duplicate data; revoked or replaced grant generations cannot publish.

Cache records preserve X-Pages, including cache hits and 304 responses omitting that header. Expiry supports Expires and Cache-Control max-age/Age. Typical official TTLs are 300 seconds for lists, 3600 for items, 300 for personal bids and 3600 for corporation bids; actual headers control scheduling. Large initial detail backlogs may span multiple conservative rate-limit windows; completion within a few minutes is not guaranteed. [CCP current OpenAPI](https://esi.evetech.net/meta/openapi.json)

## 3. Corporation sources and permissions

The smallest eligible character ID with fresh affiliation facts, required scope, usable credentials and an unblocked target provides corporation synchronization. Other characters display shared synchronization and do not duplicate list requests. If the selected source is denied or unlinked, later dispatch may use another eligible character.

Publication rechecks source membership and selection. Detected moves, expired facts or changed generations reject old requests. ESI caching still delays detection. Site administration cannot substitute for EVE consent or game-side access. A 403 blocks the affected resource while retaining credentials and unrelated role facts; confirmed invalid credentials revoke private-sync eligibility.

Unlinking or detecting an owner change removes personal contract history for that character. Corporation history remains corporation-owned. Contract read endpoints enforce current corporation-scoped authorization; a historical source relationship never grants access by itself.

## 4. Contract viewing and API

The workspace now includes `/contracts`: personal/corporation modes, authorized owner selection, literal title or ID search, type/status filters, keyset pagination, details, items and auction bids. URL parameters retain filters and page history when returning from details. See the [page design record](../ui/contracts.md).

`eve.contracts.read` admits the route only. Every list, detail, item and bid request rechecks the object: personal contracts require an active binding owned by the current account or a current site administrator viewing another member; corporation data requires `corporation.contract` against a fresh, server-resolved corporation. Existing SeAT CEO/Director/Contract_Manager mappings and manual corporation/alliance filters apply to viewing. These roles are not required for the synchronization source. Only the delivered corporation contract permission is added to the management catalog. `eve.sync.manage` grants no contract content access.

- `GET /api/v1/eve/contracts/owners`: own active characters and permitted corporations by default. Administrators can use `?member=site-user-UUID` for another member's active characters; unauthorized or unavailable members return 404. /members provides the selector; list/detail/items/bids independently recheck the current administrator flag and active target binding.
- `GET /api/v1/eve/contracts/{kind}/{id}`: 25 contracts, descending contract ID; literal title substring/exact ID via `q`, `type/status` filters and an exclusive `before` cursor.
- `GET /api/v1/eve/contracts/{kind}/{id}/{contract}`: contract and independent item/bid synchronization states.
- Append `/items?after=` or `/bids?before=`: at most 100 rows; ascending record ID or descending bid ID.

All responses use Cache-Control no-store. IDs, money and quantities are strings; absent amounts are null. Missing and unauthorized objects both return 404; invalid parameters 400, no session 401, incomplete community profile 403, unavailable services 503. DTOs never expose credentials or raw contract JSON. Authorization remains subject to ESI fact-cache latency; corporation history is denied without fresh corporation metadata.

Contracts resolve names through the injected `StaticDataService.TypeNames` service. Item names prefer local SDE Chinese, then SDE English. Missing types fall back to existing ESI name caches and IDs. `name_language` identifies the selected language; rendering items makes no per-type ESI calls. See [SDE imports](sde-names.en.md). Other public entities retain batches of up to 100 IDs via `/universe/names/`, shared caching/throttling, a one-day name cache and a three-second enrichment budget. Failures preserve contract data. Player structure resolution is described below; EVE Image Server supplies item icons. Viewing or refreshing does not resynchronize contracts.

List and detail DTOs add `summary` and `trade_direction` without changing the original ESI `title` or synchronized payloads. Summaries are generated only for blank descriptions from complete item details (items state ready). Quantities are summed per direction and type_id: one type displays name × quantity; multiple types display the first name and distinct type count; both sides distinguish provided and requested items. Couriers use start → destination. Batched local SDE/existing cache lookups supply names, falling back to #ID; decimal-string quantities retain precision. Incomplete items produce no summary; the UI falls back to contract type and displays the contract ID separately. Search still matches only the original description or contract ID.

`trade_direction` is issuer-relative: item exchanges/auctions with only provided items are `sell`, only requested items `buy`, and both sides `exchange`; couriers are `transport`. Missing/incomplete/empty items and unsupported types are `unknown`. The recipient column reuses acceptor/assignee: actual acceptor first, otherwise the assignee, with public contracts labeled public when neither is present; details keep both entities separate. Direction does not depend on description, price or successful name lookup, and does not describe the viewing character's cash flow. Enrichment batches reads within the authorized owner_kind + owner_id + contract_id scope. No additional permissions, migrations or synchronization requests are introduced.

Existing `/account` and `/sync` endpoints continue to expose metadata only, distinguishing list completion from detail completion. Manual synchronization still schedules lists, with items/bids dispatched by the sync pipeline. Contract creation/acceptance and reports remain out of scope; contract-list CSV export is available.

Loading performance: after session confirmation, an explicit owner in the URL allows owners and the list/detail read to run concurrently. Each endpoint retains its independent authorization, and the UI waits for owner confirmation before rendering data. Default owners still come from the owners response. Pointer entry or keyboard focus on a contract entry can prefetch its detail; prefetch and normal reads share user/scope/object/filter query keys and the existing 30-second freshness window. No new API, permission or cache lifetime is introduced. Synchronization, SQL and ESI name-resolution cache/timeouts are unchanged.

### List export

The frontend sequentially uses the existing protected list API with the selected owner and submitted q/type/status filters, starting from the first page regardless of the current UI cursor. Administrators selecting a member's character retain the same object checks. Every page is authorized on the server; this adds no all-members export capability, permission entry, HTTP endpoint, migration or private synchronization job. Names and summaries reuse list responses and SDE priority.

Exports are limited to 10,000 contracts. Oversized results, invalid cursors and any failed page abort the entire download; no partial file is delivered. Cancellation, actor/owner/filter changes and leaving the list abort active requests and downloads. Synchronization can update data while pages are read; this is not a database transaction snapshot.

CSV uses a UTF-8 BOM, headers in the current interface language (Chinese by default) and CRLF records. Type/status labels follow the selected language; original descriptions, names and server-generated summaries remain unchanged. Delimiters, quotes and newlines are escaped, and formula-like cells receive a text prefix. IDs and amounts retain source strings without JavaScript Number conversion; timestamps retain API ISO 8601 values. Import ID/amount columns as text in Excel to avoid its automatic numeric precision loss. The file contains contract-list fields, separate original descriptions, summary titles and recipients; it excludes item/bid rows, tokens and raw ESI JSON.

## 5. Upgrade and validation

Stop the old API, back up the database and existing token key, run `npm run db:migrate` through Goose 17 and the River migration line, then start the matching API. Migrations make no ESI requests: Goose 16 repairs completed item state and 17 establishes corporation business scope and fences excluded details. New workers cancel old excluded deliveries; deployment maintenance may also cancel precisely matched excluded jobs through River JobCancel. Missing-scope blocks remain unchanged.

Tests cover page checkpoints, independent details, cached/304 page counts, exact money, blueprints, replay, history retention, corporation source selection, changed affiliation, generation fencing and 403 isolation. Browser fixtures cover both contract resources, detail counts and 320px layout. Environment-specific results and local integration status are recorded in [project status](../project-status.md).

SeAT informed the list/detail split and item/auction scheduling; the Go implementation uses local transactional pagination, source selection and generation fencing rather than copying PHP. [Pinned SeAT corporation contract job](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Jobs/Contracts/Corporation/Contracts.php)


## Location names (2026-09-17)

Contract locations now distinguish NPC stations and player structures. Station name batches are separate from party IDs; unresolved NPC stations fall back to `/universe/stations/{station_id}/`. Structures use `/universe/structures/{structure_id}/` with the existing `esi-universe.read_structures.v1` scope and the game structure ACL. Personal contracts use their owner; corporation contracts use only the currently selected contract sync source, never unrelated members’ tokens. Binding, game ownership, grant generation and corporation source are checked before and after lookup. Successful responses use the shared encrypted ESI cache, isolated by character/generation, never the public name table. Enrichment shares a three-second primary budget with up to three workers per location class; brief rate deferrals honor the returned time. 403/404 lookups have a one-minute process-local cooldown by character/generation/location, bounded to 512 entries; this is neither an authorization grant nor a success cache. Unavailable sources, denied access and request failures preserve the original ID without asserting destruction. Lists, details, courier summaries and CSV reuse the same resolution. No migration or contract synchronization schedule change is required.

Official endpoints checked against [CCP OpenAPI](https://esi.evetech.net/meta/openapi.json).
