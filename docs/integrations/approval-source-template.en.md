# Approval source integration template

Use this checklist when adding an approval flow. The source module keeps ownership of the original record, rules, detail and write operations; approval stores only a list summary and projection state.

## Registration

Register one `reviewqueue.Source` in the host composition:

| Field | Requirement |
| --- | --- |
| `ID` | Stable lowercase identifier; never duplicate another source |
| `Capabilities` | Declare decisions, filters and `DetailKind` |
| `Access` | Return the actor's current source scope without leaking objects |
| `IndexAccess` | Provide only when index scope differs from the context scope |
| `Snapshot` | Return list summaries without tokens, full contracts, accounting or full audit data |
| `Query` | Legacy aggregation for fallback only; an index-first source may use `IndexOnly` |
| `Decorate` | Add actor-specific actions only; never write or call SDE per row |

Host startup runs `reviewqueue.ValidateSources` and fails closed when access, snapshot, legacy query (unless `IndexOnly`) or detail capability is missing.

## Summary and event shape

Snapshots populate only `reviewqueue.Item` list fields. If a source emits an event/outbox record, include `source`, `source_id`, `source_version`, event type, timestamp and a minimal summary. Reject older versions. Never include tokens, full contracts, complete reward contents or accounting entries.

## Permission checklist

- Register only the real business permission in `ManageableCatalog`; `approval.self` is session access, not business authority.
- Recheck actor, object ownership, bindings, corporation scope and self-review rules in access, detail and decision paths.
- Menu visibility never grants permission. Revoked access or unbound objects must fail on the next read/write.
- Administrators cannot approve their own record or review their own cancellation.

## Fixture and acceptance tests

Reuse `internal/platform/reviewqueue/testfixture` to cover registration, index-only reads without legacy fan-out, version protection, revoked access, self-review exclusion, cancellation/history pagination, isolated source failures, and source-owned detail/decision authorization. A new source should only register its adapter and implement snapshot/detail/decision; the central paging and query code stays unchanged.
