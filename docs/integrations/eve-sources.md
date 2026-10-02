# 官方来源与核验记录 / Official sources and verification

Verified on 2026-09-13. URLs and observations are a dated reference, not permanent defaults. 官方文档、实时元数据与本项目设计建议应区分使用。

| ID | 官方来源 / Official source | 用途 / Purpose |
| --- | --- | --- |
| S1 | [ESI overview](https://developers.eveonline.com/docs/services/esi/overview/) | Compatibility dates and authentication |
| S2 | [API Explorer](https://developers.eveonline.com/api-explorer) | Interactive endpoint reference |
| S3 | [OpenAPI, reviewed compatibility date](https://esi.evetech.net/meta/openapi.json?compatibility_date=2026-08-18) | Paths, scopes, roles, pagination and cache/rate extensions |
| S4 | [ESI best practices](https://developers.eveonline.com/docs/services/esi/best-practices/) | User-Agent, caching, error-limit headers |
| S5 | [ESI rate limiting](https://developers.eveonline.com/docs/services/esi/rate-limiting/) | Bucket identity, floating windows and response headers |
| S6 | [Cursor pagination](https://developers.eveonline.com/docs/services/esi/pagination/cursor-based/) | before/after cursor handling |
| S7 | [Pagination change announcement](https://developers.eveonline.com/blog/changing-pagination-turning-a-new-page) | Cursor ordering and end-of-list semantics |
| S8 | [EVE SSO](https://developers.eveonline.com/docs/services/sso/) | Registration, authorization and JWT claims |
| S9 | [SSO discovery metadata](https://login.eveonline.com/.well-known/oauth-authorization-server) | Current authorization/token/JWKS/revocation URLs |
| S10 | [SSO endpoint and refresh-token changes](https://developers.eveonline.com/blog/sso-endpoint-deprecations-2) | Form bodies, token rotation and SSO throttling; historical notice |
| S11 | [2026 legacy endpoint retirement](https://developers.eveonline.com/blog/spring-cleaning-legacy-routes-removed-24-march-2026) | Current OpenAPI and health endpoints |
| S12 | [Static data documentation](https://developers.eveonline.com/docs/services/static-data/) | JSONL/YAML, build discovery, change files and HTTP caching |
| S13 | [Static data portal](https://developers.eveonline.com/static-data) | Official distribution |
| S14 | [SDE build metadata](https://developers.eveonline.com/static-data/tranquility/latest.jsonl) | Latest build and release date |
| S15 | [SDE schema changelog](https://developers.eveonline.com/static-data/tranquility/schema-changelog.yaml) | Breaking field/file changes |
| S16 | [SDE rework announcement](https://developers.eveonline.com/blog/reworking-the-sde-a-fresh-start-for-static-data) | Old bsd/universe layout no longer applies |
| S17 | [Inspected JSONL archive](https://developers.eveonline.com/static-data/tranquility/eve-online-static-data-3503375-jsonl.zip) | Archive inventory and record samples |
| S18 | [Inspected build changes](https://developers.eveonline.com/static-data/tranquility/changes/3503375.jsonl) | Previous-build link; changed keys, not replacement rows |
| S19 | [OAuth security BCP, RFC 9700](https://www.rfc-editor.org/rfc/rfc9700.html) | Supplemental protocol security guidance |
| S20 | [Older official JWT reference](https://docs.esi.evetech.net/docs/sso/validating_eve_jwt.html) | Historical owner/azp/tenant claims and issuer forms; no longer maintained |
| S21 | [pgx](https://github.com/jackc/pgx) | PostgreSQL COPY and connection toolkit |
| S22 | [River transactional enqueueing](https://riverqueue.com/docs/transactional-enqueueing) | Atomic local writes and job insertion |

## 实测记录 / Observations

- OpenAPI fetched with requested date `2026-09-12` resolved to `info.version = 2026-08-18`. A subsequent request pinned to `2026-08-18` was inspected. Base server: `https://esi.evetech.net`; paths such as `/characters/{character_id}/assets` have no `/latest` prefix or trailing slash in that specification. This is specification inspection, not a live test of authenticated routes.
- `Accept-Language` includes `zh` and `en`; tenant header is `X-Tenant`, default `tranquility`. Each route still determines whether useful translated text is available.
- Build metadata: `buildNumber = 3503375`, `releaseDate = 2026-09-10T11:09:04Z`. ZIP HEAD reported `99,102,935` bytes. This compressed size is not a disk-space or RAM estimate for an import.
- Used HTTP Range to inspect the ZIP directory and the first records of `_sde`, `types`, `groups`, `categories`, `mapSolarSystems`, `npcStations`, and `translationLanguages`. Did not download/extract the complete archive or import it into PostgreSQL.
- `_sde.jsonl` matched build metadata. `types.jsonl` contains `_key`, `groupID`, a multilingual `name` object and `published`. First sample ID `0` is unpublished and lacks some numeric fields: do not require all keys to be positive or all optional fields to exist.
- `mapSolarSystems.jsonl` sample ID `30000001` contains `regionID`, `constellationID`, `securityStatus` and names including `en = Tanoo`, `zh = 坦欧`.
- First `npcStations.jsonl` sample contains location/owner/type/operation fields but no direct `name`; importer must not assume every file has a localized name field.
- Build changes `_meta.lastBuildNumber = 3500372`; the inspected change record lists keys under `added` and `changed`, rather than complete replacement objects.
- Discovery returned issuer `https://login.eveonline.com`, authorize `/v2/oauth/authorize`, token `/v2/oauth/token`, JWKS `/oauth/jwks`, revoke `/v2/oauth/revoke`, and PKCE method `S256`.
- Public GET `https://esi.evetech.net/universe/systems/30000001` with requested compatibility date `2026-08-18`, tenant `tranquility` and language `zh` returned HTTP 200, system_id `30000001`, name `坦欧`, Content-Language `zh`, ETag, Last-Modified and Expires. The response's route compatibility date was `2020-01-01`; old error-limit headers were present. This was an actual HTTP check, not a private authorization test.

## 文档差异处理 / Documentation discrepancies

The SSO guide's prose, examples, and current discovery differ in issuer spelling/trailing slash. Use an exact, reviewed allowlist; do not copy an example typo or use substring matching. Discovery fields about ID-token algorithms are not an access-token algorithm policy. Review JWKS and validate actual test tokens before release.

For GET cache revalidation use `If-None-Match`, supported by the OpenAPI and caching guide. A rate-limit page reference to `If-Match` must not be used as a substitute.

旧分页博客只用于解释 page/X-Pages；新接口以当前 OpenAPI 的游标定义为准。旧 SSO 资料只补充历史兼容背景，不覆盖当前元数据。

## 未验证 / Not verified

No registered app, real login, private ESI query, refresh/revocation flow, complete SDE integrity check, PostgreSQL import, rollback, or 4C4G load test was performed. Serenity endpoints and authorization were not established. Source checks do not imply these integrations already work.

## SDE 名称导入复核 / Type-name verification (2026-09-14)

[官方元数据 / Official metadata](https://developers.eveonline.com/static-data/tranquility/latest.jsonl) 返回 / returned build **3503375**, releaseDate `2026-09-10T11:09:04Z`。[固定 JSONL ZIP / Pinned archive](https://developers.eveonline.com/static-data/tranquility/eve-online-static-data-3503375-jsonl.zip) 为 99,102,935 字节 / bytes；本地 SHA-256 `ae0c9a8f2b695f3d425fa32db3f6087e6c8e7094e016d9c7f78b147a30a182d2`（一致性记录，不是官方签名 / local consistency record, not an official signature）。

ZIP `_sde.jsonl` 构建匹配，`types.jsonl` 成功导入 52,999 个类型，其中 52,585 个有 `zh`。样本 / samples：34 `三钛合金` / `Tritanium`；587 `裂谷级` / `Rifter`；12005 `伊什塔级` / `Ishtar`。导入器未过滤 unpublished 或 ID 0。运行说明 / operations：[中文](sde-names.zh-CN.md)、[English](sde-names.en.md)。
