# EVE ISK wallets

> Status index (2026-09-23): this business capability is deployed; remaining extensions and live verification gaps are tracked in the [backlog](../backlog.md). See [current project status](../project-status.md) for versions. Dated/Goose introduction notes describe their historical stage, not today’s release state. Apply current repository migrations, not only the initial module version.

Production terminology correction on 2026-09-20 (frontend `v0.1.0-terminology-20260920`): all 162 journal labels now come from official SDE accountingEntryTypes in both languages, including ESS Escrow Payment. Ten context labels use client localization; two retain English protocol descriptions. See [terminology maintenance](eve-terminology.en.md). ESI codes, filter values and historical records are unchanged; see [production verification](../project-status.md).

2026-09-19 bilingual data update: business requests send `Accept-Language: zh-CN|en`; the server selects presentation language in request context and returns `Content-Language` and `Vary: Accept-Language`, retaining private-response `no-store`. Local SDE type/system names prefer the requested language, with the other language, existing type-name cache or ID as fallback. Generated contract summaries and registered server messages follow the locale. Player names/notes/descriptions, business IDs, permissions, prices, content tokens and ESI caches remain unchanged. No new migration, scope or SDE import; local only. See [interface language](../ui/language.en.md).

2026-09-19: Chinese/English UI labels cover all 162 `ref_type` values and 12 `context_id_type` values in the [official ESI OpenAPI, compatibility date 2026-08-18](https://esi.evetech.net/meta/openapi.json?compatibility_date=2026-08-18). Filters and stored records retain official codes; unknown values remain verbatim. Lists show the localized type and player reason; details preserve the full original description, raw type and context. Player content is not translated. See [interface language](../ui/language.en.md) for preference and SDE-name boundaries. No API, authorization, sync-window or migration changes.

Local implementation added on 2026-09-16, Goose 33/34. `/wallet` displays character and corporation division balances, journals and market transactions. It does not issue game payments. **Future reimbursement delivery verification will use contracts, not automatic wallet-to-reimbursement matching.** Exchange continues to own shell-coin balances.

## Enable and authorize

Back up the database and configuration, run `npm run db:migrate` for Goose and River, append `wallet` to the existing `MODULES` list, retain `identity,eve,access`, rebuild both applications and restart the API. Existing login scopes already include all three wallet/division scopes; credentials missing them require owner reauthorization.

Character reads require an active binding, matching game owner, usable credential and `esi-wallet.read_character_wallet.v1`. Owners can read their own characters. Current site administrators can open another member's wallet from Members; their administrator flag is checked on each request. Permission-management and sync-management capabilities do not grant global member data access.

Corporation reads require `corporation.journal` or `corporation.transaction`, plus the selected `corporation.wallet_first_division` through `corporation.wallet_seventh_division`. Balance/name reads are restricted to authorized divisions. These delivered abilities now appear in ManageableCatalog; asset divisions remain hidden. Existing SeAT policy is unchanged: Accountant maps to journal/transactions, Account_Take_1…7 to divisions; CEO/Director/current site administrator can access known corporation scope. Junior_Accountant may supply ESI data but does not automatically gain website wallet access.

## ESI collection

The source is the [official OpenAPI specification](https://esi.evetech.net/meta/openapi.json?compatibility_date=2026-08-18), using the project's pinned compatibility date. Cache durations below are endpoint baselines; scheduling respects actual response expiry and shared rate limits.

| Resource | GET endpoint | Cache | Pagination |
| --- | --- | --- | --- |
| Character balance | `/characters/{id}/wallet` | 300 seconds | None |
| Character journal | `/characters/{id}/wallet/journal` | 3600 seconds | `page` / `X-Pages` |
| Character transactions | `/characters/{id}/wallet/transactions` | 3600 seconds | Skip an included cursor boundary; stop when no earlier rows remain |
| Seven corporation balances | `/corporations/{id}/wallets` | 300 seconds | None |
| Corporation division journal | `/corporations/{id}/wallets/{division}/journal` | 3600 seconds | Each division, then pages |
| Corporation division transactions | `/corporations/{id}/wallets/{division}/transactions` | 3600 seconds | Each division, then `from_id` |
| Custom division names | `/corporations/{id}/divisions` | 3600 seconds | Only custom names are present |

Corporation data sources need `esi-wallet.read_corporation_wallets.v1` and fresh Accountant / Junior_Accountant / Director / CEO facts. Names need `esi-corporations.read_divisions.v1` and Director / CEO; the UI falls back to “Division N”. Unqualified characters do not send corporation wallet requests.

Resource IDs: `wallet_balance`, `wallet_journal`, `wallet_transactions`, `corporation_wallet_balance`, `corporation_wallet_journal`, `corporation_wallet_transactions`, `corporation_wallet_divisions`. Shared River jobs use kind `eve.wallet-resource.v1`, the existing ESI client, cache and token buckets. One eligible credential per corporation/resource supplies data; blocked credentials allow another eligible source to take over.

One page is fetched per job. Division/page/cursor, grant generation and earliest expiry are persisted. Publication checks the credential generation and lease/fence, and rechecks corporation membership/roles before committing. A changed page count restarts scanning without deleting collected history. Network I/O stays outside publication transactions.

Journal endpoints expose the last 30 days. Local upserts distinguish record ID and payload fingerprint: different payloads sharing an ID remain separate observations and are not automatically summed. They retain previously collected older entries; they cannot recover history outside ESI's initial window. Transactions follow the upstream window without promising unlimited history. Observations retain source-credential/owner provenance: credential deletion cascades to that source's observations; scope removal or owner transfer makes its data unreadable. A replacement eligible source can recollect data still available upstream. This is not a permanent accounting archive.

## Read API and presentation

`GET /api/v1/wallet/context?member=...` returns readable owners, divisions and categories; foreign member selection is administrator-only. `GET /api/v1/wallet/records` takes owner kind/ID, division and part. Journals support reference type, direction, party ID, description/reason/record ID search and dates. Transactions support buy/sell, party ID, transaction ID and dates. Pages contain up to 50 rows; `before` is a composite `recordID.fingerprint` cursor for journals and a decimal-string record ID for transactions. `from` is inclusive and `until` exclusive; the UI includes the entire locally selected end date. See [OpenAPI](../../api/openapi.yaml).

The workbench personal-wallet figure uses `GET /api/v1/wallet/personal-corporation-summary?corporation_id=...&from=...` to aggregate the latest balance and current-month income/expense for valid bound characters of the selected corporation. The server includes only characters that still belong to that corporation, whose current credential owner matches, whose credential is usable, and which have `esi-wallet.read_character_wallet.v1`; the caller must also have the current site's `access.members.read` capability. This is a corporation-wide aggregate, accepts no arbitrary member selector, and does not return per-character detail. `from` must be within the past 45 days. It reads local snapshots only; for duplicate journal observations, only the latest variant contributes. Missing balance is `null`; ISK values and the corporation ID are decimal strings. Personal-wallet detail pages keep their existing signed-in-account/member read behavior.

The corporation operations dashboard uses `GET /api/v1/wallet/corporation-summary?owner_id=...&from=...` for readable corporation divisions and the `personal-corporation-summary` endpoint for the selected corporation's member personal wallets. Both return the latest balance plus current-month income and expense. The server rechecks corporation journal access, member-read authorization, current membership, valid bindings and wallet scope; missing access, snapshots, or synchronization remain unavailable rather than becoming zero. These are read-only local-snapshot endpoints with no payment mutation, ESI bypass, migration, or new scope.

Identifiers and ISK values remain decimal strings. Missing amounts are not zero. First/second party meanings vary by reference type, so the UI does not universally label them as sender/recipient. Unmapped official reference values remain visible. Details retain description, reason, context and journal reference; `journal_ref_id=-1` is not a valid link. Static type names prefer SDE; party names use cached names and, when needed, a bounded public ESI batch request, falling back to IDs.

UI refresh reads local snapshots and does not bypass official cache expiry. Existing rows remain visible during same-wallet refresh. The sync management page displays all seven resource types.

## Ownership and rollback

EVE's private store owns `eve_wallet_observations` and `eve_wallet_cursors`. The wallet module exposes read-only routes via host-injected EVE/static-data services, without cross-module private-store imports. No new account-level coin data exists; account merging transfers character bindings without duplicating game ISK records.

Disabling `wallet` stops dispatching its resources and snoozes existing jobs without removing data. Before rolling back to a binary unaware of the job kind, stop workers, cancel unfinished `eve.wallet-resource.v1` jobs and block wallet targets. Goose 34 down refuses to collapse multiple variants of a reference; Goose 33 down removes wallet observations; restore only from a verified backup. Local startup scripts are not production deployment instructions.

Current verification and live-test limits are recorded in [project status](../project-status.md); layout decisions are in [wallet UI](../ui/wallet.md).

Wallet resources lacking game roles recheck every five minutes. Their target/run outcome is deferred, with no failed-attempt increment and no wallet request. Historical run outcomes from before this adjustment remain unchanged.

The operations dashboard now uses `GET /api/v1/wallet/personal-corporation-income-trend` for the same valid bound-character scope. It aggregates local personal-wallet income, expense, and members with income by UTC calendar month for a six-month trend; no ESI request is made and duplicate journal observations count only once.

The finance card uses `GET /api/v1/wallet/corporation-finance-trend?owner_id=...&from=...&until=...` for the administrator's readable corporation divisions. It returns monthly income, expense, tax, and net change from local snapshots. The UI uses the corporation wallet balance as the total, shows the current UTC month's tax, and plots separate six-month wallet-net and tax trends. Decimal amounts remain strings; the endpoint is read-only and reuses the corporation wallet authorization checks.
