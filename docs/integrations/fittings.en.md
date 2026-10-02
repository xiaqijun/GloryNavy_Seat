# Fitting library and saving to EVE

> Status index (2026-09-23): this business capability is deployed; remaining extensions and live verification gaps are tracked in the [backlog](../backlog.md). See [current project status](../project-status.md) for versions. Dated/Goose introduction notes describe their historical stage, not today’s release state. Apply current repository migrations, not only the initial module version.

2026-09-19 bilingual data update: business requests send `Accept-Language: zh-CN|en`; the server selects presentation language in request context and returns `Content-Language` and `Vary: Accept-Language`, retaining private-response `no-store`. Local SDE type/system names prefer the requested language, with the other language, existing type-name cache or ID as fallback. Generated contract summaries and registered server messages follow the locale. Player names/notes/descriptions, business IDs, permissions, prices, content tokens and ESI caches remain unchanged. No new migration, scope or SDE import; local only. See [interface language](../ui/language.en.md).

Redesign deployed as v0.1.0-fitting-library-20260915 on 2026-09-15. [中文](fittings.zh-CN.md). This replaces the simulation workspace guide.

## Workflow

Current site administrators import EFT into a corporation-scoped site library, replace or delete versioned fits with auditing. This is a site library, not a claim that ESI synchronizes in-game corporation fittings. Members select their own actively bound character and explicitly confirm saving via `POST /characters/{character_id}/fittings/`. Personal fittings remain EVE snapshots and reflect successful writes after the next cache-aware synchronization. Administrators can read member snapshots via `/fittings?member=<site-account-uuid>`, but cannot write to another member's character.

Corporation skill managers can generate a skill plan from a library fit. Pinned SDE requirements recursively include hull, fitted equipment, ammunition, drones/fighters and prerequisite skills, deduplicated at the highest required level. Spare cargo modules are excluded. The shared editor allows renaming, changing levels, adding/removing skills before saving and immediate re-editing afterwards. A link opens the saved plan in skill management. These are minimum prerequisites, not an optimal training recommendation or an EVE training queue update.

The fitting page no longer exposes simulation, slot editing, CPU/PG/DPS or the Dogma worker. Legacy draft tables, compatibility APIs and engine sources remain; the current page does not load simulation WASM/SDE assets.

## Authorization and writes

All endpoints require session, fittings.self and object checks; mutations require CSRF. Import/update/delete require the current site administrator flag. Visible corporations reuse the host's active membership, site administrator scope and existing corporation skill management scope. Skill-plan writes independently enforce corporation.skills. No delegated corporation fitting write capability is introduced.

With fittings enabled in MODULES, SSO appends `esi-fittings.write_fittings.v1` to the 57-scope SeAT compatibility baseline (58 total). Enable that scope in the EVE developer application; existing grants require explicit reauthorization. The browser is shown missing scope status. Credentials stay in eve; fitting services receive a host-injected, token-free business callback.

A short transaction reserves a write before network I/O, followed by result recording. Account-scoped UUID request_key detects cross-object reuse. An account/character/library/version combination is saved at most once. States: sending, saved, failed, unknown. Explicit user retry is allowed only after a confirmed failure. Lost responses, timeouts and incomplete success responses are unknown and cannot be blindly resent. Users must inspect EVE; there is currently no automatic reconciliation or force-retry UI. A new library version can be saved separately; deleting the in-game fit does not clear the site's idempotency record.

Writes share the ESI budget and observation pipeline but never use response caching or conditional cache headers. Only HTTP 201 with a positive fitting_id is successful. Scope and ownership are checked before requesting ESI. Database transactions never span the network call.

## EFT and static reference

Import accepts official English/Chinese type names, empty slots, offline suffixes, item stacks and inline ammunition. Name: 1–50 characters; description: up to 500; items: 1–512. Unknown types are rejected. Inline ammunition without quantity requires an explicit cargo stack `Ammo Name xQuantity`; the importer does not invent magazine amounts. The game payload does not preserve offline state. CPU/PG and complete fitting legality are not simulated; EVE remains the final validator.

`internal/modules/fittings/reference.json` is generated from official SDE build 3503375, with published names, slot effects and all six requiredSkill/level attribute pairs. Display names still use StaticDataService, preferring local Chinese, with batched hull lookups. This reference and the skill catalog are pinned; validate them together when upgrading. Database name updates do not replace the embedded reference.

From repository root: `python scripts/fitting-reference.py <official-sde-jsonl.zip> --build <sde-build>`; replace angle-bracket placeholders.

## Storage, API and upgrade

Goose 29 adds fittings_library, fittings_library_audit and fittings_game_saves. Goose 27 drafts/snapshots are retained. Each corporation supports up to 300 library entries. Skills owns skill plans; eve owns credentials, snapshots, caching and budgets. All cross-module access uses injected services.

API prefix `/api/v1/fittings/library`:

| Method/path | Purpose |
| --- | --- |
| GET /context | Visible corporations, own characters/write authorization, administrator flag |
| GET ?corporation_id= | Library list |
| POST / | Administrator EFT import with corporation_id, request_key, eft, description |
| GET /{id} | Visible library entry |
| PUT /{id}, DELETE /{id} | Administrator replacement/deletion with corporation_id and current version |
| GET /{id}/requirements | Recursive skill prerequisites and SDE build |
| POST /{id}/save-to-game | version, character_id, request_key; returns state and fitting_id |

See [OpenAPI](../../api/openapi.yaml). Site IDs/versions are decimal strings; ESI payload numbers follow its official schema. Conflict: 409; hidden/unauthorized object: 404; invalid input: 400. ESI outcome is represented by result state.

Back up local database and .env, run `npm run db:migrate` (Goose 29 plus River), keep fittings in MODULES and skills for plan creation, restart API and run `npm --prefix web run build`. Add the developer application scope and reauthorize the character before real game-save testing. Rollback retains new tables, but reverting to the old synchronization binary first requires stopping workers and blocking/cancelling unsupported fitting targets/jobs; see [deployment](../deployment.md). Goose 29 Down deletes the library, audits and save records. Production is on Goose 29 with fittings/skills enabled. All three snapshot resources synchronized for seven characters. The user confirmed the developer application write scope; existing characters still need reauthorization. CCP presented a human-verification challenge to automation, which stopped there. Player sign-in and real EVE writes are not claimed as verified.

Sources: [ESI OpenAPI](https://esi.evetech.net/meta/openapi.json?compatibility_date=2026-09-14), [official fitting guide](https://developers.eveonline.com/docs/guides/fitting/), [SDE](https://developers.eveonline.com/docs/services/static-data/). [UI record](../ui/fittings.md).
