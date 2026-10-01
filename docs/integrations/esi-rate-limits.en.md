# ESI official rate-limit review

> Status index (2026-09-23): this business capability is deployed; remaining extensions and live verification gaps are tracked in the [backlog](../backlog.md). See [current project status](../project-status.md) for versions. Dated/Goose introduction notes describe their historical stage, not today’s release state. Apply current repository migrations, not only the initial module version.

Reviewed 2026-09-14. See [中文](esi-rate-limits.zh-CN.md). The review is retained below, with a 2026-09-15 implementation follow-up. Production verification is recorded in project-status.md.

## Sources and snapshot

The [OpenAPI for our compatibility date](https://esi.evetech.net/meta/openapi.json?compatibility_date=2026-08-18) was downloaded and all method/path operations inspected. It declares 233 operations: 171 with x-rate-limit and 62 without. All 46 declared groups currently have a 15m window, with different capacities. This is a dated observation, not a constant to hardcode. The undated specification returned info.version=2020-01-01, whereas the specified date returned 2026-08-18; omitting compatibility selects the oldest available behavior ([versioning](https://developers.eveonline.com/docs/services/esi/overview/)).

| Routes | Group | Tokens | Window |
| --- | --- | ---: | --- |
| Character contract list/items/bids | char-contract | 600 | 15m |
| Corporation contract list/items/bids | corp-contract | 600 | 15m |
| Character corporation roles | char-detail | 600 | 15m |
| Corporation member roles | corp-member | 300 | 15m |
| Character location/online/ship | char-location | 1200 | 15m |
| Fleets | fleet | 1800 | 15m |
| Regional market orders | market-order | 12000 | 15m |
| Character notifications | char-notification | 15 | 15m |

POST /characters/affiliation, GET /characters/{character_id}, and GET /corporations/{corporation_id} have no x-rate-limit declaration. This is distinct from missing runtime headers after a cache hit or network failure. An artificial unclassified bucket is not a CCP bucket.

## Semantics

The [rate-limit documentation](https://developers.eveonline.com/docs/services/esi/rate-limiting/) keys buckets by group and caller identity. Sibling list/items/bids routes share capacity. Capacity returns progressively according to individual consumption times, not as a full refill after the last request. Status-based costs are 2/1/5/0 for 2xx/3xx/4xx/5xx, except 429; actual metering reads X-Ratelimit-Used. Some internal game limits may produce 429 without new bucket headers.

[Best practices](https://developers.eveonline.com/docs/services/esi/best-practices/) distinguish fixed error windows from sliding bucket windows and cache expiry. In our contract endpoints the cache TTL is 300 seconds for lists, 3600 for items, and 300/3600 for character/corporation bids. These are separate from the 15m budget window. Fresh local cache avoids a request; 304 is still a network response.

## Implementation follow-up (2026-09-15)

Goose 20 addresses placeholders, double charging and quiet-window recovery: the reviewed catalog identifies declared groups; concurrent reservations settle using actual Used and expire individually. Missing measurements retain conservative reservations. Remaining only tightens local availability, never claiming live CCP balance. Valid Retry-After has no artificial 60-second floor.

Monitoring distinguishes policy source, next local expiry and measured endpoint comparison. Undeclared/unobserved routes do not create artificial buckets. See the [client guide](esi-client.en.md#official-catalog-and-per-request-budget-goose-20-2026-09-15) for catalog updates, cache rules, migration and rollback.
