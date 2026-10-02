# Interface language

2026-09-23: language switching now explicitly reloads after preserving the route, business query parameters, fragment and existing history state. This fixes same-document navigation on anchored URLs: both directions create a new document without a second manual refresh.

2026-09-20: wallet, role, contract and slot names now use the source-backed `eve-terminology` catalog, bypassing application-copy translation. Site currency uses Nutshell Coins consistently. See [current terminology maintenance](../integrations/eve-terminology.en.md) for resolved findings and provenance.

Local implementation, 2026-09-19. Simplified Chinese is the default; English is also supported. The Corporate Clean design remains unchanged.

The top bar and standalone sign-in page offer a language icon with `EN` or `中文`. The button identifies the target language, has a localized accessible name and a minimum 44px hit area.

Switching **reloads the current URL**, preserving its path, query and fragment. This keeps module-level labels, lazy routes and dialogs consistent. In-memory filters and unsaved form inputs do not survive the reload; save or close forms first. The browser stores the choice in `glorynavy.locale`; it does not change accounts, permissions, SSO or ESI scopes. If storage is unavailable, a `lang=zh-CN|en` URL parameter is used; new URLs without that parameter default to Chinese.

Navigation, fixed labels, actions, states, dialogs, chart descriptions, registered API errors and date/number formatting use the selected language. `html.lang` is updated. Business time zones and exact ISK string formatting are preserved.

Character/corporation names, player notes, fitting names, contract descriptions and historical audit content remain unchanged. SDE type, skill, ship and solar-system names prefer the selected language, falling back to the other stored language, existing ESI type-name cache or ID. Station/player structure names retain their ESI source names. Switching requires no SDE import or additional ESI requests. Unknown API messages and future enum values remain visible in their original form.

## Development

Keep game terminology separate from application copy. An official ESI enum validates the protocol code, not our handwritten Chinese or English display label. Do not claim unverified labels are official client wording. Continue using SDE names for types, skills, ships, systems and existing generated categories; record client or official localization sources and versions for other game terms. See the [historical audit](game-terminology-audit-2026-09-20.md) for original findings and the [terminology guide](../integrations/eve-terminology.en.md) for their resolution and source maintenance. Passing mapping tests does not establish translation accuracy.

Use `msg("Chinese source", ...values)` and `getLocale()` from `web/src/lib/i18n.ts`. Add English entries to `web/src/lib/locales/en.ts`; source text is the stable key. Prefer complete sentences with `{0}`, `{1}`, etc. over concatenated fragments. Preserve placeholders. Empty translations are allowed for language-specific grammatical fragments; only missing keys fall back to source text.

Translate fixed labels including accessible names, placeholders and errors. Never translate protocol values, permission IDs, ESI enum codes or user input before submitting them. Business decisions must use stable data values. `APIError` translates only registered fixed messages and preserves unknown text.

Do not mutate React-owned DOM text or use a MutationObserver for translation. No machine-translation requests are introduced. A future reactive language switch would also need to replace the module-level label constants; re-rendering the shell alone is insufficient.

Tests: `web/src/lib/i18n.test.ts` covers placeholders, preferences, storage failure and original content; `web/e2e/wallet.spec.ts` covers round-trip switching, sign-in, wallet dialogs, raw notes and narrow layouts. See [project status](../project-status.md) for this delivery's actual results.

[简体中文](language.md)

## Server presentation boundary

Use apiFetch for business requests and the platform locale context on the server. Background work defaults to Chinese. The locale does not affect ESI fetches/cache keys. Update the UI catalog and run `node scripts/backend-messages.mjs` (`--check` verifies the generated Go subset). Message is for known server text, never user input. Skill/ship group_english values come from the matching official SDE build and reference generators.

Welfare list/detail responses project only display names and fixed valuation reasons from authorized snapshots. Stored evidence, audit entries, unknown fields, exact numbers and content tokens remain unchanged. A failed name lookup retains the stored name. Business rules continue to use IDs/codes.
