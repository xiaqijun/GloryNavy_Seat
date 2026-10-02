# Official EVE terminology catalog

> Status index (2026-09-23): this business capability is deployed; remaining extensions and live verification gaps are tracked in the [backlog](../backlog.md). See [current project status](../project-status.md) for versions. Dated/Goose introduction notes describe their historical stage, not today’s release state. Apply current repository migrations, not only the initial module version.

Deployed to production on 2026-09-20 in frontend `v0.1.0-terminology-20260920`, against Tranquility build 3503375. See [release verification](../project-status.md). The earlier audit missed localized `accountingEntryTypes.jsonl` and `corporationRoles.jsonl` in the official SDE. Contract and slot labels were also checked against the matching installed Tranquility client's localization resources. This is a pinned catalog, not a claim of automatic updates to future client wording.

## Sources and scope

| Domain | Entries | Source |
| --- | --- | --- |
| Wallet journal | 162 | SDE accountingEntryTypes; exact ESI `internalName` mapping; both languages use `name` |
| Corporation roles | 55 | SDE corporationRoles via explicit `shortName` mapping, including 28 division roles and Terrestrial Logistics Officer |
| Contracts | 5 types / 10 statuses | Client ContractsWindow labels and Generic/Unknown |
| Equipment locations | 10 | Client Ship, InfoWindow, Killmails and Common labels, shared by fittings and attendance |
| Wallet contexts | 12 | 10 client labels; `eve_system` and `industry_job_id` retain English protocol descriptions EVE system / Industry job, without inventing official Chinese |
| Legacy growth hulls | 4 | SDE types 17715, 12005, 29990 and 22448; fallback names only, never overwrite custom project names |
| Capital classes / legacy statistics | 2 / 8 | Client Industry, Fitting and Common labels; no restoration of the retired simulator page |

ESS uses the official **ESS Escrow Payment / 事件监测装置保证金支付**. Absolution's fallback Chinese name is corrected to 救赎级. Player names, fitting/project names and historical evidence remain untouched.

Contract status mappings were also checked against `contractscommon.GetContractStatusText` constant/label references in the same client: `finished_issuer` is ItemsNotYetClaimed, `finished_contractor` is UnclaimedBySeller, `finished` is Finished, and `reversed` is Reversal. Partial completion stays partial; filters, exports and business logic continue using the unchanged ESI codes. The existing legacy filter group conveys loan compatibility separately from its official display name.

The site's currency is consistently **Nutshell Coin / Nutshell Coins**. This is application terminology, as are welfare workflow states, PAP, appraisal directions and sync states.

## Reproducible generation

[Source mappings](../../scripts/eve-terminology-sources.json) connect protocol keys to records/labels; they contain no handwritten Chinese names. The [generator](../../scripts/eve-terminology.py) requires a Tranquility client matching the SDE build, validates resource sizes and MD5 against resfileindex, and records SHA256. Pickles accept primitive data only; executable globals and persistent references are rejected. Game code is not executed.

The [generated catalog](../../web/src/lib/eve-terminology.json) stores bilingual names, source records or client label/message IDs and hashes. Full client resources, raw pickles and machine-specific paths are not committed. The [display helper](../../web/src/lib/eve-terminology.ts) selects names by domain, code and language, falls back to existing English when Chinese is unavailable, and preserves unknown codes. Game names bypass the application's copy dictionary. Shared slot formatting preserves indices and unfamiliar flags.

```sh
python scripts/eve-terminology.py <official-jsonl.zip> --client <Tranquility-tq-directory> --resources <ResFiles-directory>
python scripts/eve-terminology.py <official-jsonl.zip> --client <Tranquility-tq-directory> --resources <ResFiles-directory> --check
```

Replace placeholders with local paths. `--check` regenerates and compares the complete artifact. Review source mappings on updates, then run the check, frontend tests/build and affected UI checks. Unit tests cover behavior, not independent proof of official translations.

This is a build-time terminology catalog. It does not expand the database SDE importer or modify SDE scheduling/version pinning. No additional ESI calls, scopes, reauthorization or migration are needed. Deploy matching frontend assets; this production release leaves the backend and database versions unchanged.

Validation: 83 frontend unit tests, lint/build, source regeneration and backend-message consistency checks passed. Ten focused desktop/mobile wallet, contract and role checks passed, including original codes, language switching, long labels, status/filter behavior and authorization changes. Old role-label assertions were updated and rerun. This is not a full browser-suite acceptance run. CCP owns the source labels; see [notices](../third-party-notices.md).

[中文](eve-terminology.zh-CN.md)
