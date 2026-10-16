# Scheduled character cleanup by corporation

[中文](corporation-cleanup.zh-CN.md) · [Character rules](seat-multi-character.en.md) · [Members UI](../ui/members.md) · [Current status](../project-status.md)

## Configuration and scope

Current site administrators configure **Cleanup rules** on `/members`: up to 50 corporation IDs separated by commas or new lines. Preview before enabling and saving. Defaults are disabled with an empty list; enabling an empty list is rejected. Each save of an enabled rule schedules a run seven days later; completion schedules the next run seven days after completion. Disabling clears the schedule. Timestamps use UTC in storage and the browser timezone/current language in the UI. There is no immediate cleanup endpoint.

Evaluate each character separately, including external alts of members with an allowed main, main characters, last characters and administrators' characters. Preview provides removal/retention/uncertain counts and the first 100 removal candidates, without deleting anything. Execution rechecks membership. Every read, write and preview requires the current site administrator flag; site roles and EVE roles do not substitute. PUT/POST require Origin and CSRF; stale configuration versions return 409.

## Removal and retained records

- Require an active binding, matching game owner, unexpired profile and role snapshots with matching corporations, ready credentials with corporation-role scope, and current-generation profile/authorization targets with a successful observation, zero failures and neither blocked nor failed state. Missing, expired, failed, inconsistent or ownership-blocked facts are skipped.
- Remove bindings, local credentials and their dependent private caches/online observations, personal contract caches and sessions authenticated by the character. Clear pending authorization flows and affected pending merge requests. No CCP remote token revocation or in-game item action is performed.
- If the main character is removed, choose the lowest-ID remaining active character. With no active character left, clear the main and revoke all account sessions. Retain accounts, site grants, community details, loans/debts, PAP/coin ledgers, applications and delivery snapshots. Manual self-unlink still protects main/last characters.
- Preserve removed character ownership and owner hashes. A new SSO proof by the same owner restores the original account, keeping an existing main or setting the returning character as main when none exists. Ordinary cross-account linking and different-owner takeover are rejected. Explicit account merge moves preserved ownership while retaining original_user_id; concurrent changes reject restoration so the user must restart login.
- This is weekly unlinking, not a corporation admission gate. A new SSO proof can restore a character; if it still falls outside the list, a later cycle removes it again.

## Background execution and migration

Goose 90 adds identity-owned policy, preserved ownership and audit tables. The host injects EVE `CleanupMembership`, `RemoveCharacterTx` and current administrator checks into identity, without cross-module private-store access. River registers `identity.corporation-cleanup.v1` on `eve_control`: a minute-level due scan processes at most 25 accounts per job. The existing eve module/runtime is required; there are no new environment variables or cron entries.

Each account uses one transaction: exclusive policy lock, character advisory locks in ID order, account lock, roster recheck, credential locks, membership verification/removal and durable cursor advancement. Credential locks follow sync publication ordering; no network requests run in the transaction. Failures roll back the current account; retries/restarts resume the cursor without repeating committed accounts. Saving a rule resets the old cursor and schedule but does not undo completed removals. Audits record configuration versions, removals and completion counts. The UI distinguishes the last completed result from a current run.

Endpoints: `GET/PUT /api/v1/identity/corporation-cleanup` and `POST /api/v1/identity/corporation-cleanup/preview`; see [OpenAPI](../../api/openapi.yaml). Errors: 400 `invalid_cleanup`, 401 unauthenticated, 403 administrator/CSRF rejection, 409 `cleanup_conflict`, 503 `cleanup_unavailable`.

Before rollback, disable cleanup and cancel unfinished `identity.corporation-cleanup.v1` River jobs. Once preserved ownership exists, do not run older binaries that ignore it: returning characters could create incorrect accounts. Down migration refuses while preserved ownership exists. Recovery requires the matching release and pre-release backup; do not selectively delete ownership or business history.

## Verification boundary

See project status for database and mocked browser checks. A production administrator must choose the allowlist, review the preview and save it before the first live cycle. Inspect real audit records and session invalidation after that cycle; fixtures do not constitute live character cleanup acceptance.
