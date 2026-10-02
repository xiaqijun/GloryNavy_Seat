# SeAT-style corporation authorization

> Status index (2026-09-23): this business capability is deployed; remaining extensions and live verification gaps are tracked in the [backlog](../backlog.md). See [current project status](../project-status.md) for versions. Dated/Goose introduction notes describe their historical stage, not today’s release state. Apply current repository migrations, not only the initial module version.

2026-09-16: local wallets are delivered. ManageableCatalog now includes corporation.journal, corporation.transaction and wallet divisions 1…7. Reads require both category and division permissions. ESI source roles are checked separately; current administrators may read active members’ wallets. See [wallet authorization](wallet.en.md).


Goose 32 raw loss reads require ownership of an active bound character or current site-administrator status, with corporation and binding checks. `corporation.welfare` permits review of submitted case evidence, not all other members' raw losses. Names and QQ/KOOK never establish ownership; removed scope, invalid authorization or changed game owner blocks old evidence. See [character losses](character-losses.en.md).

Goose 31 locally adds the `welfare.self` entry capability and configurable `corporation.welfare` for review/delivery. The latter requires an explicit scoped grant or current site-administrator status; CEO/Director game roles do not imply it. Policy/history configuration, direct coin grants and reversals remain administrator-only. Members act on their own applications; even administrators cannot approve or deliver their own claims. Object reads continue to check active bindings and corporation scope; welfare authority does not grant contract or ESI data access. See [welfare](welfare.en.md). Not deployed to production.

Updated: 2026-09-14. [中文](seat-authorization.zh-CN.md).

Update: login now requests the 57 current-compatible scopes from SeAT's default profile, with separate `/login` and `/account` pages. Use [the current scope setup](seat-login-scopes.en.md), not just the earlier role-only scope below. The corporation policy and synchronization boundaries remain applicable.

## Permission management UI (2026-09-14)

`/access` provides role configuration, member assignments and audit history. Navigation uses `can_manage` from `/access/me`; every management API still enforces `access.manage`. Administrators and roles containing that ability can manage all site roles and account assignments. EVE CEO/Director status does not confer site administration. When the community module is enabled, complete QQ/KOOK details on `/account` first; external confirmation is not required.

- Roles now expose a decimal-string `version`. PUT requires `version: "0"` to create or the last-read version to update. DELETE requires `?version=<current version>`. Stale changes/deleted objects return 409 without an audit event. Existing management clients must update their write requests.
- The management catalog exposes delivered access/sync management, contracts, attendance, skills, welfare and wallet journal/transaction/division abilities. Undelivered corporation features and asset divisions remain hidden. Internal SeAT mappings in `Catalog` and historical grants remain compatible. Editing a role preserves grants outside the published catalog. Global `access.manage` cannot carry entity filters.
- GET `/api/v1/access/members?q=...&after=...` searches any bound character name (case-insensitive literal substring), exact character ID or account UUID. It returns `items/next`, up to 25 accounts in UUID order, with main-character display information, character count, administrator flag and site roles. Reset the cursor when the search changes. No community details or credentials are returned.
- GET `/api/v1/access/audit?before=...` returns `items/next`, up to 50 events in descending event-ID order. Events retain actor, action, subject ID and time, not historical display names or before/after diffs. Existing user-role PUT/DELETE endpoints perform assignment and revocation.
- The host composes identity's member directory with access-owned role queries. Private stores remain isolated. A main character is display information; assignments belong to the account and corporation scopes remain independent.

Run `npm run db:migrate` for migration `00009_access_management.sql`, then restart the API and publish the matching frontend. The foundation marker remains 1. Initial administrator access still requires an explicit operator command for an existing signed-in character; the first user is never automatically promoted. Local personnel assignments are audited in the database, not hardcoded.

The role-management UI and audit reader are delivered. Squads, role inheritance, permission explanations, restricted administrative delegation and full audit diffs remain future work. Granting `access.manage` grants global role management, not administration limited to one corporation.

## Delivered scope

Implemented: character corporation-role synchronization, a SeAT-style corporation policy, independent site RBAC, management APIs, and authorization status on the character page. Squads, corporation-wide member-role ingestion, and asset/financial ingestion remain separate work. A permission catalog entry does not mean its business module is implemented.

The behavior follows SeAT's [CorporationPolicy](https://github.com/eveseat/web/blob/master/src/Acl/Policies/CorporationPolicy.php), [role mapping](https://github.com/eveseat/web/blob/master/src/Acl/EsiRolesMap.php), [character role job](https://github.com/eveseat/eveapi/blob/master/src/Jobs/Character/Roles.php), and [authorization documentation](https://eveseat.github.io/docs/admin_guides/authorizations/), implemented in Go.

## Policy

An authenticated request is checked against known abilities. Site administrators can use registered abilities. For a target corporation, its CEO or a Director among the user's active, fresh character bindings receives corporation abilities only. Other game roles use the SeAT mapping. Explicit site-role grants and corporation/alliance filters are then checked. Otherwise access is denied. Unknown abilities remain denied even for administrators.

Accountant maps to summary, journal and transaction; Junior_Accountant and Auditor to summary; Contract_Manager to summary and contract; Diplomat to summary and tracking; Security_Officer to summary and security; Trader to summary and market; Project_Manager to projects. Account_Take_1–7 and Container_Take_1–7 map to their respective wallet and asset divisions. Ordinary membership alone does not grant corporation summary access.

Only the `roles` array grants default abilities. HQ, base and other-location roles are stored separately and do not become global roles. Grantable roles are not held roles. CEO comes from `ceo_id`. The referenced mapping's second-container index and contract/project names are corrected to their intended abilities and ESI enum values.

An empty corporation/alliance filter means unrestricted entity scope, matching SeAT 4+. The two filter categories are OR conditions. `access.manage` is global and cannot have entity filters. Game IDs cross JSON boundaries as decimal strings. A target's alliance must come from server-side data, never the request body.

## Configuration

Keep the existing local Client ID and Secret. Add this scope to the **existing EVE application**:

```text
esi-characters.read_corporation_roles.v1
```

Both the registered application's allowed scopes and the player's consent must include it; old scope-free sessions are not automatically upgraded. See the [official SSO documentation](https://developers.eveonline.com/docs/services/sso/). The callback remains:

```text
http://127.0.0.1:5173/api/v1/eve/callback
```

Enable `MODULES=system,identity,eve,access,community`, keep the existing EVE credentials and PUBLIC_ORIGIN, then run:

```sh
npm run auth:key
npm run dev:external
```

The key command generates `EVE_TOKEN_KEY` in `.env` only when missing or empty. It never prints or replaces an existing key. This must decode to 32 bytes and is required when eve and EVE login are configured. Back it up securely with the database. Online key rotation is not implemented; replacing the key makes existing credentials unreadable and requires new consent.

Open `/login` and use the normal EVE login button, or **授权军团职务** with an existing session. Initial synchronization is transactionally queued in River, subject to backlog and upstream availability. Resource refresh requests enqueue work without bypassing ESI caching.

## Storage and synchronization

`eve` owns SSO, encrypted credentials and snapshots; `access` owns site roles, assignments and audit records; `identity` supplies active bindings and owner hashes. The host composes token-free DTOs without crossing private SQL boundaries.

Access and refresh tokens are encrypted together with AES-256-GCM. Random nonces and authenticated character/owner binding prevent ciphertext substitution between identities. Tokens are never returned to browser APIs. The worker reads public affiliations, public corporation details and the authenticated character-role endpoint using compatibility date `2026-08-18`.

Public and encrypted private cache, conditional requests and cross-instance limits now use PostgreSQL. See the [ESI operations guide](esi-sync.en.md).

River now owns dispatch and recovery, with generation/lease fencing. Fetches occur outside publication transactions; token rotation has a separate ten-second lock budget. The old poller is removed. There is no cross-system exactly-once guarantee.

Rotated tokens commit before subsequent ESI calls. Temporary failures back off with jitter. A 401 allows one justified refresh; 403 blocks the resource without deleting credentials. Confirmed invalid grants or scope loss require reauthorization. Successful synchronization replaces all role groups. Repeat logins cannot bypass cache waits.

Game-derived privileges expire five minutes after the next upstream cache expiry if no new snapshot arrives; cached authorization trust is limited to two hours. Site-role grants remain independent. ESI caching and polling mean departures, role changes and authorization revocation are not detected instantly. Every protected request checks freshness server-side. Logging out revokes only the local session; ESI consent remains active until revoked separately through EVE Authorized Applications and detected by a subsequent request.

On a detected corporation change, the old snapshot is deleted and new roles are confirmed only on the next cycle after endpoint caches expire. This avoids carrying a cached old-corporation role into the new corporation.

## Administration

After the intended administrator has signed in once, a local operator explicitly selects their character:

```sh
npm run access:admin -- --character YOUR_CHARACTER_ID
npm run access:admin -- --character YOUR_CHARACTER_ID --revoke
```

No account is created or chosen automatically. The command resolves an active binding and audits the change. Management APIs require site-administrator status or an `access.manage` site-role grant. Mutations additionally require exact Origin and the session's X-CSRF-Token.

| Endpoint | Purpose |
| --- | --- |
| GET `/api/v1/access/me` | Own game-role facts, site roles and administrator status |
| GET `/api/v1/access/catalog` | Configurable abilities for delivered features; currently access.manage only |
| GET `/api/v1/access/roles` | Site-role list |
| PUT / DELETE `/api/v1/access/roles/{id}` | Save/delete a UUID role |
| PUT / DELETE `/api/v1/access/users/{user}/roles/{id}` | Assign/remove a role for a UUID user |
| GET `/api/v1/access/corporations/{id}/summary` | Snapshot protected by `corporation.summary` and entity scope |

Role PUT bodies contain `name`, `grants` and `version` ("0" for creation). Each grant contains `permission`, `corporations` and `alliances`; filter IDs are strings. For example, a grant for `corporation.journal` with `corporations: ["123456"]` is restricted to that corporation. Generate the role UUID client-side. Find the signed-in user's UUID in `/api/v1/identity/session`. Changes record actor, action, subject and time; an audit UI is delivered; Squads membership history remains future work.

Migrations 00004 and 00005 bring Goose to 5; the foundation marker remains 1. Local database/browser tests use isolated fixtures and simulated ESI. Real role consent, callback and game data remain to be verified after the registered scope is updated.

## Administrator member data viewing (2026-09-14)

Site administrators can search /members, select a member's bound characters and read community details, site roles, corporation role snapshots, ESI sync status and synchronized personal contracts/items/bids. Every request checks the current account administrator flag. Ordinary access.manage/eve.sync.manage grants or in-game roles do not grant this cross-member access. The acting account's community completeness gate remains in effect.

GET /api/v1/access/members/{user}/data returns display characters, access facts and QQ/KOOK values plus confirmation states. Community is null when its module is disabled. Invalid UUID: 400; unavailable member: 404; non-administrator: 403. Responses use no-store. access.members.read is a registered administrator-only ability, excluded from assignable role catalogs and rejected in role grants. Demotion rejects subsequent requests immediately; administrator identity is not cached.

Personal contract reads and GET character sync status admit administrators for other members' active bindings. Missing, unlinked or blocked characters return 404. Own account/session, linking, unlinking, main-character selection, reauthorization, community edits and self POST refresh retain ownership rules. This does not extend ordinary CEO/Director personal-data access, add ESI scopes/migrations/sync jobs or expose credentials. See the [member page record](../ui/members.md).

### Delivered attendance capability

`corporation.attendance` is now configurable with corporation/alliance filters; existing CEO/Director and current site-administrator rules apply. Fleet capture uses the organizer's own active character and ESI fleet access. Administrator member-online reads check the current flag and active bindings without expanding token/account writes. [Scope details](attendance.en.md).

## Corporation skill requirements

The delivered corporation.skills capability permits plan maintenance and requirement-only checks for actively bound characters in its corporation scope. Full skill/queue reads remain owner/current-site-administrator only. Backend object authorization is always required. See [skills authorization](skills.en.md).
