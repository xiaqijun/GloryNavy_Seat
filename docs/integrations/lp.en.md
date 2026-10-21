# LP exchange lookup

`/lp`, under Tools, is a read-only NPC loyalty-store comparison. It never creates site orders, debits LP or coins, issues rewards, creates contracts or enters approvals. No personal LP scopes or balances are used. Independently implemented from the behavior of glory-navy 955c2ded; no unlicensed source or assets copied.

Select a faction, NPC corporation and one of Jita, Amarr, Dodixie, Rens or Hek. Search Chinese/English names or IDs; sort and paginate 25 offers per page. All non-deleted NPC corporations come from the active SDE; corporations without offers show an empty state. Player corporations are excluded.

Profit = output value × (1 − fees) − store ISK − required items − manufacturing materials. Output uses either the lowest sell or highest buy order at the chosen station; input materials use lowest sell orders. Blueprints use SDE manufacturing recipes, ME 0 and ESI runs; installation, hauling and order-book depth are excluded. Missing recipes/prices never count as zero costs. Fees accept 0–100%, initially 0. LP and AK are ranked separately; mixed-currency offers have no single-point rate. Losses remain visible.

## API and boundaries

All routes require a session, the host profile gate and `lp.self`. This is not a delegable management permission. GET `/api/v1/lp/catalog` returns the active SDE build, bilingual NPC corporations and factions. GET `/api/v1/lp/offers?corporation=1000130` validates against this catalog and uses the shared, token-free ESI gateway and its cache/budgets for loyalty offers (20-second budget). Names are batch-resolved through StaticDataService.

GET `/api/v1/lp/prices?region=10000002&types=34,35` accepts five fixed station mappings and at most 100 positive type IDs; the UI requests sequential batches of 20. Fuzzwork is tried first (5 seconds, fixed HTTPS destination, no redirects, 2 MiB response limit). On failure, shared ESI reads the same station’s orders: three concurrent requests, at most 20 pages per type, stable pagination, 20-second budget. Confirmed 404 means no market listing; other failures invalidate the batch. Two concurrent batches, duplicate-request coalescing, a one-minute upstream cooldown and a maximum 256-entry / 60-second cache bound the work. ESI cache lifetime is capped at the earliest source expiry. Sources and oldest observation time are returned. The UI waits for complete pricing before showing profits; failed batches are retryable.

Invalid input is 400, anonymous access 401, profile rejection 403, a disabled module 404, busy 429 (Retry-After: 5), unavailable source 503. No upstream body, user identity, cookies or EVE tokens are forwarded to Fuzzwork.

LPDataService verifies the active imported SDE ZIP’s SHA-256 and reads only NPC corporations, factions and blueprints (32 MiB/file, 1 MiB/line, duplicate/required-file/CRC checks). It rechecks the active release after reading and caches only that version. Automatic SDE updates and operator pins control this data too; the existing names importer is unchanged.

## Configuration

Append `lp` to existing MODULES, keeping identity/access/eve dependencies. Deploy matching frontend/backend. No schema migration, private store, scheduled job, account-merge participant, key or new ESI scope is required. The active official ZIP must remain in SDE_WORK_DIR; missing/mismatched data fails closed.

Optional LP_HTTP_PROXY configures only Fuzzwork’s trusted HTTP(S) proxy (development example: http://127.0.0.1:7890). Empty uses the environment; invalid configuration fails closed. Shared ESI proxy settings are unaffected. Disable the module and roll back both artifacts if necessary; retain the database and public ESI cache.

Sources: [CCP SDE](https://developers.eveonline.com/static-data), [ESI](https://developers.eveonline.com/docs/services/esi/overview/), [Fuzzwork](https://market.fuzzwork.co.uk/api/). See [project status](../project-status.md) and [page layout](../ui/lp.md).
