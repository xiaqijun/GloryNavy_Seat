# SeAT default scopes and standalone login

> Status index (2026-09-23): this business capability is deployed; remaining extensions and live verification gaps are tracked in the [backlog](../backlog.md). See [current project status](../project-status.md) for versions. Dated/Goose introduction notes describe their historical stage, not today’s release state. Apply current repository migrations, not only the initial module version.

With fittings enabled, the application appends `esi-fittings.write_fittings.v1` to the 57-scope SeAT baseline (58 total). This project-specific game fitting write scope does not change the baseline list below. Enable it in the developer application and explicitly reauthorize existing characters. See [fittings](fittings.en.md).

Updated: 2026-09-14. [中文](seat-login-scopes.zh-CN.md). This guide supersedes the earlier role-only scope setup.

## Scope source

The requested scopes follow SeAT's default `sso_scopes`, pinned to [eveseat/web cd112870](https://github.com/eveseat/web/blob/cd11287006bddbf00c48cf13fd1fa704b0ea25d6/src/WebServiceProvider.php). Like its [SSO controller](https://github.com/eveseat/web/blob/cd11287006bddbf00c48cf13fd1fa704b0ea25d6/src/Http/Controllers/Auth/SsoController.php), the request omits `publicData` when private scopes are included.

SeAT's configuration contains 59 entries. Removing `publicData` and `esi-characters.read_chat_channels.v1`, which is absent from the current [ESI OpenAPI catalog](https://esi.evetech.net/meta/openapi.json?compatibility_date=2026-08-18), leaves **57 scopes**. All remaining entries have been compared with the pinned source. SSO discovery does not publish `scopes_supported`; its absence does not imply that scopes are unsupported.

The profile covers character/corporation information, assets, wallets, contracts, markets, skills, industry, mail and notifications. It also retains SeAT's `esi-ui.open_window.v1`, so the entire profile must not be described as strictly read-only. The application now provides multiple synchronization modules and explicitly confirmed fitting writes; scopes never automatically trigger game actions. Broader consent does not implement additional data modules or grant unrestricted site RBAC permissions.

[Complete scope list](seat-default-scopes.txt). The request's source of truth is `internal/modules/eve/scopes.go`. This release has one built-in default profile; SeAT's multi-profile administration and inherited main-character scopes are not implemented. Explicit linking now uses the current local profile; see [multi-character support](seat-multi-character.en.md). Browsers cannot submit arbitrary scope lists.

## Setup and pages

1. Allow the listed 57 baseline scopes and, when fittings is enabled, `esi-fittings.write_fittings.v1` on the **existing EVE application**. Preserve the existing credentials and encryption key.
2. Keep callback `http://127.0.0.1:5173/api/v1/eve/callback`.
3. Run `npm run dev:external`, or migrate with `npm run db:migrate` before restarting a separately deployed API.
4. Open [the standalone login page](http://127.0.0.1:5173/login) and consent through CCP. Both the application's registered scopes and the player's consent must match, as described by [official SSO documentation](https://developers.eveonline.com/docs/services/sso/).

`/login` has no workspace sidebar/top bar. Its normal state shows only the brand and official EVE button; scope details are confirmed on EVE's authorization page. A successful callback goes to `/account`; an already authenticated login visit redirects there. Cancellation and errors remain visible on the login page even when a prior session exists.

`/account` displays character identity, corporation and site roles, logout and updated consent. Anonymous visits redirect to `/login`. Existing public workspace/system-status pages are not made private in this change; business APIs retain server-side authorization.

## Validation and migration

Requested scopes are saved with each one-time login flow. The callback verifies that the signed JWT contains those original scopes, so a configuration change cannot weaken an in-flight request. Tokens remain encrypted; granted scope metadata is stored separately. `/api/v1/eve/status` returns the requested list, and `/api/v1/access/me` reports `needs_authorization` for incomplete existing grants.

Old role-only credentials are not automatically expanded. They may continue refreshing their original grant and using role snapshots subject to freshness checks, while the account page prompts for renewed consent. A refresh must preserve the credential's previously granted scopes; it need not satisfy a newer, broader global login profile.

Migration `00006_eve_scope_profile.sql` introduced scope tracking; multi-character migration is `00007`, and community migration `00008` brings the current Goose version to 8 without changing foundation marker 1. Old scope metadata starts empty and is populated by synchronization or renewed consent. Tests use isolated database/provider fixtures and browser mocks. Real CCP application configuration, player consent and callback verification remain pending; constructing a valid authorization URL alone does not verify real consent.
