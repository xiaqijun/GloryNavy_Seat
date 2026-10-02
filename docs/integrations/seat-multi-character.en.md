# SeAT multi-character reference and local design

> Status index (2026-09-23): this business capability is deployed; remaining extensions and live verification gaps are tracked in the [backlog](../backlog.md). See [current project status](../project-status.md) for versions. Dated/Goose introduction notes describe their historical stage, not today’s release state. Apply current repository migrations, not only the initial module version.

2026-09-15 update: duplicate accounts now have a separate SSO proof, preview and confirmation flow. PAP, coins and orders move with provenance; ordinary character linking still rejects ownership conflicts. Target site permissions and community profile remain unchanged. See [account merge](account-merge.en.md); earlier planned-merge wording below is historical.

Updated: 2026-09-14. [中文](seat-multi-character.zh-CN.md). Status: explicit linking, main selection, per-character reauthorization, unlinking and the character page are implemented. See current project status and backlog for live acceptance boundaries rather than reusing the initial test conclusion.

## 1. Verified reference

Reference: `eveseat/web` commit `cd11287006bddbf00c48cf13fd1fa704b0ea25d6`. Historical comments about groups do not describe the current model by themselves.

- [User model](https://github.com/eveseat/web/blob/cd11287006bddbf00c48cf13fd1fa704b0ea25d6/src/Models/User.php): a user has `main_character_id`, characters associated through refresh tokens, and user-level site roles.
- [SSO controller](https://github.com/eveseat/web/blob/cd11287006bddbf00c48cf13fd1fa704b0ea25d6/src/Http/Controllers/Auth/SsoController.php): unauthenticated login resolves a user by character and owner hash. Authenticated linking can reassign that character's token to the current user. New users start with the authenticating character as main. Linking prefers the main token's scopes.
- [Profile controller](https://github.com/eveseat/web/blob/cd11287006bddbf00c48cf13fd1fa704b0ea25d6/src/Http/Controllers/Profile/ProfileController.php): main selection checks ownership and updates the main ID and display name, with duplicate-name account handling. Unlinking deletes the user's token association. This does not merge every character or permission of two users.
- [Token observer](https://github.com/eveseat/web/blob/cd11287006bddbf00c48cf13fd1fa704b0ea25d6/src/Observers/RefreshTokenObserver.php): creation/restoration triggers character updates; deletion triggers authorization filter-data updates.

The following is our Go-module adaptation, not a reproduction of every SeAT behavior.

## 2. Member, main and alternate characters

One site user represents one member and may bind characters from multiple EVE game accounts. Each character completes SSO separately. This phase neither groups game accounts nor infers unauthorized characters.

| Entity | Rule |
| --- | --- |
| Site user | Stable UUID; owns QQ/KOOK details, community confirmations and manually assigned site roles |
| Main character | One per user with characters; initially the first login character; supplies display name, portrait and default viewed character |
| Alternate characters | Peers under the same user, each with independent corporation, roles, ESI grant and synchronization state; no nested hierarchy |
| Authenticating character | Proves identity through SSO; any valid bound character logs into the same user |
| Viewed character | Selects page data; changing it does not change the main character, rewrite authentication identity or increase permissions |

Main status grants no administration rights and does not suppress an alternate's own game roles. Access remains subject to valid character facts, target corporation and site grants; see [authorization](seat-authorization.en.md).

## 3. Operation rules

| Operation | Implemented local behavior |
| --- | --- |
| Ordinary login | Resolve an existing binding, or create a user with the new character as main; an existing browser session does not imply linking |
| Add character | Explicit authenticated action; persist `link` intent, target user, original session and requested scopes, then recheck at callback |
| Add an already owned character | Return the existing association without duplicate users, bindings or an implicit main change |
| Add another user's character | Reject the ownership conflict; do not transfer tokens, users, community details or site roles |
| Reauthorize | Persist `reauthorize` intent and expected character; reject a different SSO selection instead of linking it |
| Set main | Require a bound character with valid identity; preserve user UUID and site grants |
| Unlink an alternate | Revoke sessions authenticated by that character, remove usable credentials and authorization facts, then detach; preserve other characters' credentials |
| Unlink the main | Select another valid bound main first; ordinary unlinking cannot remove the last usable login character |

Linking and reauthorization use the current local [current scope profile](seat-login-scopes.en.md) (57 baseline, 58 with fittings), recorded in the flow. Do not copy possibly obsolete or incomplete scopes from the main token. Consent, validation and refresh remain per character.

ESI revocation preserves the binding and main marker, requires reauthorization and immediately excludes invalid role facts. Owner-hash mismatch instead blocks the character for ownership resolution and revokes its sessions; it never transfers the character automatically. An invalid main must not prevent another valid bound character from signing in.

Explicit duplicate-account merging is implemented; recovery after character sale and administrator unlinking remain separate, unavailable flows. Ordinary linking never merges accounts. Explicit merges preserve first ownership and original audit; see the account-merge guide.

## 4. Implementation boundaries

Migration `00007_identity_characters.sql` adds the main-character composite foreign key, SSO intent and `identity_character_events` audit. Existing single-character users are backfilled. Existing users with multiple characters stop the migration instead of choosing a main arbitrarily; back up such nonstandard data and prepare an explicit backfill migration rather than deleting characters to bypass the guard. Empty users cannot produce sessions.

- identity `Complete` verifies character, user and original session and commits identity, credentials and login session in one transaction. `SignIn` delegates to login intent. Linking and reauthorization preserve the original authenticating character and session expiry.
- eve persists one-use intent, target user, expected character and requested scopes. SSO exchange runs outside the database transaction. A consumed callback cannot be retried; start a new authorization after failure.
- The host injects `SaveTx` and `RemoveCharacterTx` through business interfaces. Mutations lock character, user, session and credential in that order. The user lock serializes main selection, unlinking and session-limit changes.
- access reads current valid bindings and target-scoped facts. Main selection does not increase permissions. Unlinking deletes local credentials and snapshots, revokes sessions authenticated by that character and removes unconsumed member flows. It does not call CCP's remote token-revocation service.
- Binding and credential lifetimes remain separate. ESI revocation preserves bindings; owner-hash anomalies quarantine identity. Audit keeps the original user ID, character ID, action and time; unlinking neither deletes nor transfers it.
- community now implements [shared manual details and completeness gating](community-profile.en.md). Bots and external confirmation endpoints remain pending.

| Endpoint | Behavior |
| --- | --- |
| `GET /api/v1/identity/characters` | List own bindings, including blocked identities, main first |
| `POST /api/v1/identity/characters/{id}/main` | Select an active owned main |
| `DELETE /api/v1/identity/characters/{id}` | Protected unlink; main/last-active protection returns 409 |
| `POST /api/v1/eve/characters/link` | Create link flow and return CCP URL |
| `POST /api/v1/eve/characters/{id}/reauthorize` | Create expected-character flow and return CCP URL |

Mutations require a site session, same-origin Origin and `X-CSRF-Token`. In `/identity/session`, `character` remains the authenticating character and new `main_character` supplies default display. Member-flow success returns to `/account`, with sanitized errors when needed; missing/invalid state returns to `/login`. See [OpenAPI](../../api/openapi.yaml) for response contracts.

The page defaults to the main; avatar cards switch the viewed character. Actions include disabled, pending and failure feedback. The unlink dialog supports keyboard cancellation and retry. Unlinking the authenticating character clears browser query caches and returns to login. Other unlink/main changes refresh identity and access queries.

## 5. Acceptance criteria

1. Characters from two game accounts bind to one site user and either logs into that user ID.
2. First login sets the default main. Main selection, viewed-character selection and alternate login remain distinct and preserve community details and site roles.
3. Cancellation, expiry, replay, invalid original sessions, wrong expected characters and concurrent conflicts leave no incorrect binding or credentials.
4. Cross-user conflicts preserve the original user's characters, sessions, community details and grants without transfer.
5. Alternate revocation removes only its effective ESI privileges. Corporation A roles cannot authorize corporation B; main status never elevates access.
6. Unlinking removes associated sessions and data access. Main/last-login protections run on the backend with traceable audit records.

The 2026-09-14 phase passed complete Go/isolated PostgreSQL tests, Go vet, frontend lint, 13 unit tests, a production build and 14 desktop/mobile login, authorization and multi-character browser tests, plus a corporation-scope integration test. That phase used simulated SSO and test characters, not real CCP consent; this historical test limitation does not mean EVE remains unconnected today. See current project status.
