# SDE type and solar-system name imports

> Status index (2026-09-23): this business capability is deployed; remaining extensions and live verification gaps are tracked in the [backlog](../backlog.md). See [current project status](../project-status.md) for versions. Dated/Goose introduction notes describe their historical stage, not today’s release state. Apply current repository migrations, not only the initial module version.

2026-09-19 bilingual data update: business requests send `Accept-Language: zh-CN|en`; the server selects presentation language in request context and returns `Content-Language` and `Vary: Accept-Language`, retaining private-response `no-store`. Local SDE type/system names prefer the requested language, with the other language, existing type-name cache or ID as fallback. Generated contract summaries and registered server messages follow the locale. Player names/notes/descriptions, business IDs, permissions, prices, content tokens and ESI caches remain unchanged. No new migration, scope or SDE import; local only. See [interface language](../ui/language.en.md).

Reward configuration adds local item search through StaticDataService.SearchTypes: exact type ID or literal Chinese/English name substring in the active SDE, 2–80 characters, at most 30 results, excluding type 0. No ESI request is made. The selected UI language is preferred for display (Chinese by default); administrator-configured ISK values do not come from SDE.

## Solar-system names (Goose 23, mapper 2)

The current importer reads `types.jsonl` and `mapSolarSystems.jsonl`, not the complete map. `StaticDataService.SolarSystemNames` resolves names in a local batch: the selected SDE language, then the other language, then UI `#ID`. No per-row ESI calls. Existing capture/loss system IDs remain unchanged; importing names makes historical entries readable without another capture.

Stop the local service and back up the database/configuration. Run `npm run db:migrate`, `npm run sde:import -- --latest`, then start the matching application. A mapper-1 release at the same build is upgraded to mapper 2; cached official ZIPs are reused. Automatic updates never remove an operator pin. For a pinned old release, explicitly import with `--file <official-zip> --build <build>`, or `--resume` and wait for automatic updates. Newer builds cannot be replaced by older builds.

Type and system names publish in one transaction under the same release pointer, lease/fence and pin rules. The system file is mandatory: 64 MiB uncompressed, 4 MiB per line, at most 100,000 rows; validate IDs, names, duplicate IDs, CRC, empty sets and count drops over 20%. Status adds `mapper_version` and `system_count`. Activating an old mapper-1 release falls back to IDs without mixing releases. Keep Goose 23 tables during a code rollback; do not run Down. Goose 14/15 descriptions below document the original type-only stage.

Implemented on 2026-09-14: the `type-names` profile reads `_sde.jsonl` and `types.jsonl` from the official Tranquility JSONL ZIP. It preserves `zh` and `en` names for all types, including unpublished types and ID 0. At that stage descriptions, categories, systems, recipes and market prices were excluded; system names are now included as described above; this is not the full `core` profile.

## Operation

When upgrading to Goose 15, stop the old API and back up PostgreSQL, then run from the repository root:

```sh
npm run db:migrate
npm run sde:import -- --latest
npm run start:api
```

The script loads `DATABASE_URL` from local `.env`. `--latest` reads the unique `_key=sde` record from [official metadata](https://developers.eveonline.com/static-data/tranquility/latest.jsonl), compares the active build and downloads a fixed build ZIP only when a higher build exists. Archives use `SDE_WORK_DIR` (default `.local/sde/`). Redirects must remain on the official HTTPS host. Downloads use a temporary file and rename on completion. An existing archive is reused and validated by the importer.

Import a downloaded official archive or explicitly restore a retained release:

```sh
npm run sde:import -- --file .local/sde/eve-online-static-data-3503375-jsonl.zip --build 3503375
npm run sde:import -- --activate <release-id>
```

`--activate` also pins the version and suspends automatic updates, preventing an in-flight download from overwriting it. Use the local release ID printed by the importer, not the CCP build, for activation. The build above is a verified local example. Repeating the same build, ZIP SHA-256 and mapper version returns the existing release without writing or changing the active pointer. Use `--activate` to reactivate it explicitly. SHA-256 records local consistency and is not an official signature. New imports cannot downgrade the active build.

## Name resolution

Contract items query the active SDE in a batch, with **SDE Chinese → SDE English → cached ESI name → `#ID`** precedence. ESI cache fallback applies only to missing types. English is never recorded as a Chinese translation. API `name_language` is `zh`, `en`, or an empty string. Resolution is centralized in `StaticDataService.TypeNames`, injected into contract reads. Future assets and fittings reuse this service instead of importing eve store or maintaining separate name caches.

Item rendering does not fetch individual ESI types or resynchronize contracts. Public character, corporation and station references retain the existing batch enrichment; contract player structures now use credential-scoped ESI resolution and isolated caching, preserving IDs on failure; see [contract locations](contracts.en.md#location-names-2026-09-17). Chinese names require neither new authorization nor contract resynchronization. The next request sees a committed SDE release without an additional process cache.

## Storage and recovery

Goose 13 marks existing ESI names as `en` and separates languages. Goose 14 adds `eve_sde_name_releases`, `eve_sde_type_names` and `eve_sde_active_names` to eve's private store. The operator CLI uses the eve service; no new HTTP import endpoint or configurable business permission is introduced. Goose 15 adds `eve_sde_update_state` for the durable check clock, lease, fence, pin, latest outcome and failure count.

Limits: 512 MiB ZIP, 1 MiB metadata, 1 GiB uncompressed types, 4 MiB per line, 4096 bytes per name and one million types. The metadata, types and solar-system allowlisted entries are read, without filesystem extraction. Paths, duplicate entries, build, CRC, JSON, required fields and unique type IDs are checked. Unknown fields are accepted. Empty imports and a type-count drop greater than 20% are rejected.

Downloads happen outside transactions. A transaction advisory lock serializes imports; COPY writes batches of 1000 rows. Release data and the active pointer commit atomically while the old version remains readable. Failure rolls back the whole candidate and returns a CLI error, leaving the active version intact. No partial release is retained. Automatic attempts persist their latest failure category and retry time separately; manual import errors are reported by the command. Committed releases can be reactivated; no automatic pruning runs.

Automatic updates and latest status are implemented. A complete failure-history panel, queued missing-type enrichment, other datasets and report-wide version pinning remain pending. The [earlier full SDE design](eve-integration.en.md#10-initial-sde-import) includes proposed environment variables, profiles, scheduling and states that this CLI does not implement.

## Automatic updates and status

The implementation adopts SeAT's independent SDE scheduling, shared static models and version checks that avoid repeated imports. Chinese precedence, atomic activation and explicit rollback remain project adaptations. References use eveapi commit `990a0a2`: [contract model](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Models/Contracts/ContractItem.php), [default monthly schedule](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/database/seeders/ScheduleSeeder.php), [version check](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Commands/Eve/Update/Sde.php). This project's six-hour default differs from SeAT's schedule.

| Configuration | Default | Behavior |
| --- | --- | --- |
| `SDE_AUTO_UPDATE` | `true` | Enables public SDE updates when eve runs, even without SSO credentials; false and restart disables them |
| `SDE_CHECK_INTERVAL` | `6h` | Interval after a successful check; accepts 1h–168h |
| `SDE_WORK_DIR` | `.local/sde` | ZIP cache; configure a writable persistent directory for production |

The existing 30-second due scanner feeds a separate `eve_sde` queue with one worker. Active-job uniqueness and a database lease prevent duplicate work across instances. `eve.sde-update.v1` carries no credentials and is not repeated per character. Only a higher build downloads/imports; current or older builds record `unchanged`. PostgreSQL retains the next due time across restarts. SDE can run without SSO; ESI business readiness is tracked independently.

Work budget is 15 minutes, River timeout 16 minutes and lease 20 minutes. River recovery and due scans recover interrupted work. Publishing checks fence, lease, pin and current build inside the transaction; stale workers cannot overwrite newer state. Failures back off for 5, 10, 20 minutes and onward, capped at six hours; success resets the count. River retries cannot bypass the persisted clock. Failed downloads/imports preserve published data; the separate queue leaves character and contract workers available.

```sh
npm run sde:import -- --status
npm run sde:import -- --resume
```

`--status` emits JSON with active release/build/SHA-256/type count, pin, latest check/success, result, error category, failure count and next check. Error categories are `metadata_failed`, `download_failed`, `import_failed`, and `database_failed`, without tokens or private responses. `--resume` removes the pin and makes the next check due; execution still requires a running service with `SDE_AUTO_UPDATE` enabled. Manual `--latest` does not remove a pin and skips archive verification when the build is already current; use `--file --build` to verify a local archive.

If a cached ZIP fails validation, move it aside before updating again. Retries reuse existing archives and do not silently overwrite operator-supplied files. Status is currently available through the CLI, not the `/sync` page.

## Verification

Local verification used official build 3503375: 52,999 types, 52,585 Chinese names and 414 English fallbacks. All 771 distinct contract item types matched SDE; 767 had Chinese names. Reimport returned the same release without rebuilding data.

Database regression coverage includes Chinese precedence, English and ESI-cache fallback, unresolved IDs, no per-item HTTP requests, repeat imports, rollback, ID 0 and unknown fields. Invalid builds, older builds, duplicate IDs, empty or unexpectedly small datasets, malformed JSON, oversized lines, duplicate ZIP entries and traversal paths are rejected without changing the active release or published release count.

Format reference: [Static Data](https://developers.eveonline.com/docs/services/static-data/). [Chinese edition](sde-names.zh-CN.md).

Automatic-update regression coverage also includes duplicate dispatch, unchanged-build download avoidance, persistent due times, failure backoff and preservation of last success, retained-release reactivation, rollback during download, stale-fence rejection, real River startup/shutdown without SSO, and duplicate metadata rejection.

The first local automatic check at 2026-09-14 15:54:11 (UTC+8) returned `unchanged`, retaining build 3503375 / release 1 and scheduling the next check for 21:54:11. API readiness passed. New-build transitions and failure cases were verified with fixtures.

## Fitting reference data boundary

The current fitting page no longer provides attribute simulation. Names still use StaticDataService. The Go service embeds lightweight official build 3503375 type/slot/prerequisite reference data for EFT and skill generation; this is not a full database Dogma import. Automatic name updates do not replace pinned reference data. See the [fitting guide](fittings.en.md) for regeneration. Legacy simulation dependencies remain in historical source.

## Skill category reference

Skill names still prefer StaticDataService. A lightweight compiled category/group reference and server-side allowlist use pinned official SDE 3503375. Database name updates do not replace this artifact; see [skills upgrade](skills.en.md).
