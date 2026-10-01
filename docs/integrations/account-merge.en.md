# Account merge

## Release update 2026-09-23 (deployed)

Alliance PAP snapshots participate in merge preview fingerprints and account migration, preserving the first `original_account_id` introduced by Goose 47. Snapshot changes invalidate old previews. Synchronization/manual conversion and merging share binding and active-account locks to prevent crediting retired accounts. Rebinding a character does not transfer historical entitlements.


> Status index (2026-09-23): this business capability is deployed; remaining extensions and live verification gaps are tracked in the [backlog](../backlog.md). See [current project status](../project-status.md) for versions. Dated/Goose introduction notes describe their historical stage, not today’s release state. Apply current repository migrations, not only the initial module version.

2026-09-20, local: growth allowances are account plus ship type. Under the welfare publication lock, merge preview/commit checks cross-project hull conflicts in both directions against applications, completed cases and manually recorded used history. Pending conflicts block merging; both completed histories remain preserved and still prevent future claims. No new migration; see [current welfare rules](welfare.en.md).

Goose 31 adds a [welfare participant](welfare.en.md), registered even when its UI is disabled. Before its tables exist it returns an empty summary. The preview includes welfare case count and a fingerprint of both case and qualification history. Overlapping pending reservations, including conflicts with the other account's historical claims, return `merge_business_conflict`; resolve them and refresh the preview first. Completed history remains intact, qualifications and Active months are deduplicated, and first `original_account_id` is retained; identity/history may need renewed verification. Exchange still migrates coins without new issuance. Once welfare records exist, do not roll back to a binary that omits this participant. This extension is deployed with the welfare module; see current project status.

Goose 30, deployed on 2026-09-16 with v0.1.0-account-merge-fixes-20260916. Real dual-account CCP acceptance remains member-operated; see [project status](../project-status.md).

## Flow and ownership

Sign in to the account to retain, choose Merge account on My characters, then prove control of an existing source account through EVE SSO. The callback creates a ten-minute, original-session-bound proof only. Review all source characters, the retained main, PAP, net Guoke coin balance and orders, then explicitly confirm. Cancel or expiry does not move ownership. Changed financial data requires a refreshed preview; changed character membership, owner hash, status or main requires renewed proof. Unknown characters and same-account merges are rejected; both rosters must be active and total at most 100 characters.

All source characters move. The target main, QQ/KOOK profile, confirmations and site permissions remain; source profiles, administrator status, roles and historical audit are retained on the retired source account, never copied or combined. The source account's official QQ openid binding and unused binding challenges are removed rather than copied; the target must generate a new challenge. Both accounts' other sessions and pending character authorization flows are revoked; the initiating target session survives. All moved characters subsequently sign in to the target account. This is not a sold-character recovery or a game ownership transfer.

PAP attendance, awards and ledger, coin entitlements/ledger/orders, fitting drafts and game-save idempotency records move their current account attribution. `original_account_id` preserves the first owner across repeated merges. Row IDs, amounts, original actors, dates, exchange rates and order states remain. Balances combine including reservations, spending and debt, without minting coins, changing inventory or converting historical PAP. Existing cancellation/refund and original-rate PAP correction continue to work. Corporation artifacts retain their original creators and audits; access still requires corporation scope. Ship/loss evidence follows attendance entries. Full same-member account merges preserve online samples, unlike individual character reassignment.

ESI credentials and snapshots remain; the source proof character receives the normal authorization refresh. Workers still check binding, grant generation and fences. No in-game asset transfer occurs. A game-save in `sending` blocks merging until it finishes. Account-level idempotency collisions roll the entire operation back; no records are discarded to resolve them. No self-service undo or split is provided.

## Contracts and concurrency

Identity owns proof, session binding, expiry, roster/preview fingerprints and completion audit. Host-injected module `MergeAccountTx` services run inside one database transaction, keeping SQL private. Persisted data participates even when its module UI is disabled. New account-owned modules must implement and test this contract.

Acquire ordered character advisory locks, ordered account locks and the original session lock, then re-read ownership and preview data. Move business attribution, revoke other sessions/flows, move characters and persist the source-to-target mapping and audit atomically. Failures roll back; completed same-session, same-fingerprint confirmations are idempotent. Module-owned database triggers call the identity active-account contract, taking an account lock and rejecting late writes against retired sources. Operation audit guards also prevent old in-flight writes from committing. External ESI calls never run inside the merge transaction. Lock conflicts require retrying a fresh preview.

`POST /api/v1/eve/accounts/merge` starts SSO. `GET /api/v1/identity/merges/{id}` creates a preview fingerprint without changing ownership; `POST` confirms with that exact `token`; `DELETE` cancels a pending proof. Session authentication applies throughout; writes require same-origin and CSRF. A proof UUID alone grants no access to another session. No administrator impersonation or delegated merge capability is added. See [OpenAPI](../../api/openapi.yaml).

## Upgrade and acceptance

Stop local API/workers, back up the database/configuration, run `npm run db:migrate` to current Goose and River versions (account merge was introduced in Goose 30) and start matching frontend/backend. No new module or OAuth scope and no automatic historical merges. This feature is deployed; later releases still require backups and both migration checks. Never run Down after real merges: it removes provenance and retired-account safeguards; disable the UI or retain compatible schema for rollback.

Tests cover proof/session isolation, expiry/cancellation, SSO preview-only callback, CSRF, changed previews, concurrency, transaction rollback, retired-source writes, provenance, balances, refunds and corrections. Browser fixtures do not replace real CCP and member acceptance. [中文](account-merge.zh-CN.md).
