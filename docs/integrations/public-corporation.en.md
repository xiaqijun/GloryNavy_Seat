# Public corporation profile for the home page

> Status index (2026-09-23): this business capability is deployed; remaining extensions and live verification gaps are tracked in the [backlog](../backlog.md). See [current project status](../project-status.md) for versions. Dated/Goose introduction notes describe their historical stage, not today’s release state. Apply current repository migrations, not only the initial module version.

Deployed on 2026-09-23 as backend v0.1.0-public-home-20260923; see [project status](../project-status.md) for validation boundaries. [中文](public-corporation.zh-CN.md)

## Scope and contract

Anonymous GET `/api/v1/eve/public/corporation` is fixed to Glory Navy (98530802). It accepts no target, upstream URL, path or member credentials. The 200 response contains `data`:

- corporation_id, name, ticker, member_count and date_founded are official corporation information. Member count means game characters, not people.
- Optional alliance contains id, name, ticker and corporation_count (null if unavailable). This count describes the entire alliance, not Glory Navy.
- updated_at is the corporation response validation time, falling back to content update time. stale means refresh failed and a previous successful aggregate is being returned.

An initial failure returns 503/public_profile_unavailable without upstream error details. Missing numbers render as a dash, never a fabricated zero. The public page reads public projections and the existing session endpoint, not private module directories, member details, PAP, approvals, wallets, assets or scope configuration. The first release only linked to zKillboard; the local addition below supplies activity aggregates.

## Combat and online characters (2026-09-23, deployed as public-strength-r1)

The user requested public combat and online figures, explicitly counting multiboxed characters separately. Anonymous GET `/api/v1/eve/public/activity` has a fixed corporation (98530802), no target/upstream parameters, and returns `corporation_id`, `combat`, `online`. Each source independently becomes null on initial failure.

- combat contains six UTC calendar `months[{month,kills,value}]` including the current month, `updated_at`, and `stale`. It reads the fixed [zKillboard statistics API](https://zkillboard.com/api/docs/) `/api/stats/corporationID/98530802/kills/`. Kills mean corporation participation in published reports, not exclusively final blows; ISK value is zKillboard's estimate, not income. Missing months remain null; explicitly loss-only months use zero for the omitted kill side. Other upstream identity fields/lists are not published. Ten-minute in-process cache, coalesced refresh, eight-second timeout, 2 MiB response limit, descriptive User-Agent, automatic gzip and no redirects. Failures back off one minute and preserve the original combat snapshot/time with stale=true. Refresh is demand-driven, with no persistent aggregate cache.
- online contains `characters`, `covered_characters`, `bound_characters`, `updated_at`, `expires_at`. Every character counts separately, even under the same account. Only active bindings with fresh Glory Navy profiles and matching owner hash, authorization generation, valid authorization, online scope and corporation samples within 180 seconds are counted. No valid samples means characters=null, never a fabricated zero. Coverage is sampled/linked corporation characters, not coverage of the whole corporation.
- The host injects identity bindings and EVE-owned local observations. Anonymous reads never initiate authenticated ESI calls. Online aggregates cache for 30 seconds without stale fallback; the HTTP response is no-store. The browser polls every 30 seconds and hides expired online figures. No names, IDs, locations or individual states are exposed. Existing River sampling and `esi-location.read_online.v1` suffice; no new migration, scope or setting.

This user-authorized aggregate is a narrow public projection, not a change to member-data authorization. Deploy frontend and backend together. Older backends return 404 and the new activity section degrades independently.

The final UI shows current-month numbers only, without charts, coverage explanations, methodology tooltips or source timestamps. The six-month projection remains in the API contract; reducing visual copy does not remove validation.

Online cache lasts at most 30 seconds. Expired samples trigger a new aggregate on the next request; the browser checks earlier at expires_at (minimum one second) instead of waiting the full polling interval. Original sample freshness is never extended.

## ESI and caching

The host injects the shared ESIService, using anonymous GET requests:

1. `/corporations/98530802/`
2. `/alliances/{alliance_id}/` when an alliance exists
3. `/alliances/{alliance_id}/corporations/`, counted without per-corporation fan-out.

Reference: [official API Explorer](https://developers.eveonline.com/api-explorer#/operations/GetCorporationsCorporationId).

No character token or new SSO scope is used. Requests reuse the existing compatibility date, PostgreSQL cache, ETag/Expires handling and shared rate limits. Only explicitly listed fields are projected; descriptions, CEO, tax rates and private fields are never forwarded.

The in-process aggregate coalesces concurrent refreshes. It refreshes at the earliest valid upstream expiry, with a one-minute fallback/backoff. Shared refresh has a 12-second timeout and survives individual visitor cancellation. Corporation failure preserves prior values and timestamp with stale=true. Alliance failure leaves alliance information/count unavailable. The aggregate has no separate durable table: after restart it is reconstructed from the shared ESI cache; a failed reconstruction without an in-memory snapshot returns 503.

The browser checks every five minutes; upstream requests still honor official caching. Aggregate HTTP responses cache for 60 seconds and initial errors for 30 seconds. There is no separate River job. If the existing ESI service is unavailable, recruitment and page content remain usable.

## Deployment and validation

Deploy frontend and backend together. No migration, new environment variable or session change is required. Old frontends ignore this new endpoint; new frontends with old backends show unavailable public data.

Tests cover the field whitelist, anonymous ESI requests, fixed target, aggregate caching, backoff, stale preservation and missing values. The local endpoint was verified against real public ESI data. Observed counts are not configuration defaults. Responsive layouts, both languages and reduced motion are described in the [home-page record](../ui/homepage.md).
