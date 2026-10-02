# EVE login setup and implementation

> Status index (2026-09-23): this business capability is deployed; remaining extensions and live verification gaps are tracked in the [backlog](../backlog.md). See [current project status](../project-status.md) for versions. Dated/Goose introduction notes describe their historical stage, not today’s release state. Apply current repository migrations, not only the initial module version.

2026-09-21: The three-second authorization budget covers session lookup, profile gating, permission checks and renewal only. Business handlers receive the original request deadline/cancellation and the authenticated principal, not the authorization child context. Operations such as market valuation keep their own budgets. No session migration or forced sign-in is required.

Updated: 2026-09-14. Environment: Tranquility. [中文指南](eve-login.zh-CN.md).

Implemented: standalone EVE login, sessions, role synchronization and SeAT-style authorization. With access enabled, login requests 57 scopes from the current ESI-compatible SeAT default list and stores encrypted tokens. This does not implement additional data modules. See [the current scope guide](seat-login-scopes.en.md).

## 1. Register an application

Use **My Applications** from the [official SSO documentation](https://developers.eveonline.com/docs/services/sso/). Register a confidential application whose Client Secret stays on the backend. Set this local callback exactly:

```text
http://127.0.0.1:5173/api/v1/eve/callback
```

Use the frontend origin, not internal API port 8080. Do not mix localhost and 127.0.0.1. Production uses `https://your-domain/api/v1/eve/callback` and separately configured application credentials.

The current production target callback is `https://seat.kisectool.com/api/v1/eve/callback`, with `PUBLIC_ORIGIN=https://seat.kisectool.com`. Credentials stay on the application server. Reusing the existing developer application requires updating its registered callback; the old local callback may then stop working. Use separate applications for ongoing parallel development and production. DNS, certificates and verified deployment status are documented in the [deployment guide](../deployment.md) and [project status](../project-status.md).

A separate production application is now configured; development retains its original application. Production credentials reside in `/etc/glorynavy/seat.env` on the application server. Restart `glorynavy.service` after replacing them and preserve `EVE_TOKEN_KEY`. Configuration presence and correct redirect parameters do not prove that the Client Secret has passed a real authorization-code exchange.

The production proxy serves HTML with `Referrer-Policy: same-origin`; API responses and the callback retain `no-referrer`. A page-level `no-referrer` policy can cause native login forms to send `Origin: null`, correctly rejected as `csrf_failed`. Fix the page policy and reload instead of relaxing server-side origin validation. Deployment checks must click the real login button; see the [deployment checks](../deployment.md).

## 2. Configure the application

Edit the root `.env`:

```dotenv
MODULES=system,identity,eve,access,community
PUBLIC_ORIGIN=http://127.0.0.1:5173
EVE_CLIENT_ID=your-client-id
EVE_CLIENT_SECRET=your-client-secret
EVE_TOKEN_KEY=
```

PUBLIC_ORIGIN has no path or trailing slash. The callback is derived from it. HTTPS is required except for loopback development. Never place credentials in VITE variables, frontend code, or version control.

With both credentials empty, the server runs and the login page says it is not configured. Partial credentials fail startup validation. A configured status checks presence only, not whether CCP accepts the credentials.

Allow the [57 listed scopes](seat-default-scopes.txt) on the existing application. Run `npm run auth:key` only if the key is missing; preserve existing credentials and keys. Restart with `npm run dev:external` or `npm run dev` for Docker; both migrate first. Separately deployed APIs need migration before restart. The [standalone login page](http://127.0.0.1:5173/login) leads to `/account` after success; anonymous account visits return to `/login`.

## 3. Flow and protections

The browser posts a same-origin form. The backend creates a ten-minute, single-use state linked to an HttpOnly browser cookie, the prior local session, and a PKCE S256 verifier. It retrieves and caches trusted discovery metadata and JWKS. The callback atomically consumes the matching flow before exchanging the code with HTTP Basic client authentication and PKCE.

The JWT library verifies RS256 signatures, exact issuer, time claims, both required audiences, character identity and owner; azp and tenant are checked when present. Invalid cookies, changed sessions, expired states, and replayed callbacks are rejected. Token and credential contents never appear in responses or request logs. Local redirects use fixed error codes and no-referrer/no-store headers.

An owner change blocks the character binding and revokes its sessions. It does not transfer the old account's privileges. Administrative review/unblocking UI is not implemented yet.

## 4. Sessions and module boundaries

`identity` owns accounts, character bindings and sessions. `eve` owns login flows, encrypted credentials and role synchronization. `access` owns site roles and authorization policies. The host injects the business services. Cookies are HttpOnly, SameSite=Lax and Secure on HTTPS, without a Domain attribute. Session credentials are stored only as hashes. Sessions have a seven-day sliding lifetime (local change, 2026-09-17). Same-site session reads and authenticated business requests passing entry permission and CSRF checks renew expiry and the cookie. Frequent requests coalesce renewal writes to once per five minutes; `expires_at` reports the actual stored expiry. Existing unexpired twelve-hour sessions upgrade on activity without signing in again; expired, logged-out and revoked sessions cannot be revived.

Renewal preserves the token and CSRF value and does not extend SSO state, merge previews or EVE token lifetimes. At most five sessions remain per account; a new login still rotates the browser session. Cross-site probes, entry permission/CSRF failures and logout do not renew; ESI background jobs without browser requests do not count as activity. The session endpoint also aligns cookie lifetime with stored expiry to recover from a lost renewal response. Linking/reauthorizing does not directly reset login expiry; normal browser activity uses the renewal policy above.

The session endpoint returns anonymous status or the authenticating `character`, display `main_character`, expiry and CSRF token. Logout requires an exact Origin, an active session and X-CSRF-Token, revokes the session, clears cookies and discards frontend caches. Access now applies site roles, game-role mapping and entity filters; unknown abilities remain denied. The management UI is implemented; Squads remain deferred. Explicit multi-character linking is implemented; any valid bound character can sign into the same user. Link/reauthorize preserve the original session and expiry. Explicit account merging is implemented; see [account merge](account-merge.en.md) and [multi-character behavior](seat-multi-character.en.md).

Login does not verify QQ or KOOK membership. The community module implements manual details and a completeness gate for protected business operations. Confirmation records remain separate; bot/event integration is pending. See [community details](community-profile.en.md).

Login migrations are 00002 and 00003; authorization adds 00004 and 00005, and scope tracking adds 00006, and multi-character support adds 00007, and community details add 00008. These are initial login-related migrations; see project status for the current Goose version. The foundation marker remains 1, and obsolete SQL drafts were removed. Expired flows/sessions are unusable and opportunistically cleaned in batches of 1000. Login starts are limited to 20 per direct peer IP per ten minutes. Forwarded headers are not trusted; production proxy deployment must address trusted client IP handling or revise the shared quota.

## 5. Verification and remaining work

Login and private ESI are in real use. Early simulated provider/ESI tests establish only those scenarios. Additional scopes still require player reauthorization; live fitting writes, contract fulfillment and the latest production login-form check are tracked in the [backlog](../backlog.md). Verify identity, session persistence, logout, cancellation/retry, browser-tab isolation and corporation-specific access with the actual registration.

See the [authorization guide](seat-authorization.en.md) for implemented encryption, refresh and role policy, and the [full integration guide](eve-integration.en.md) for other ESI data and SDE. This phase performs no game writes.
