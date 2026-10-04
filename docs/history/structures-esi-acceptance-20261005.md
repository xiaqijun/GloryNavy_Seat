# 建筑管理真实 ESI 验收记录（2026-10-05）

本轮按 STRUCT-01 验收项执行，使用本机隔离 PostgreSQL 与 Tranquility 官方端点；没有接触生产数据库或生产令牌。

## 已通过

- Tranquility `GET https://esi.evetech.net/latest/status/` 返回 HTTP 200。
- 官方 OpenAPI 当前仍要求：
  - `/corporations/{corporation_id}/structures` 使用 `esi-corporations.read_structures.v1`；
  - `/corporations/{corporation_id}/starbases` 和详情端点使用 `esi-corporations.read_starbases.v1`。
- 未授权请求访问三个私有建筑端点均返回 HTTP 401。
- 本地 Goose/River 已迁移到 Goose 68；`internal/modules/eve` 的 ESI 缓存/授权集成测试通过，结构模块与应用测试通过；前端 27 个测试文件、94 项测试通过。

## 未完成

本机数据库中有 6 个历史授权记录，均已记录建筑 scope，但其访问令牌需要刷新。使用当前本地 EVE 应用配置执行真实角色快照时，6 个角色均返回 `EVE SSO request or verification failed`（`sso_unavailable`），因此没有取得可核对的真实建筑列表、POS 多建筑燃料、Upwell `fuel_expires` 或授权失效响应。

因此 Director/CEO 与受限角色的军团范围、POS 燃料数量、缓存陈旧和 ESI 失败场景仍不能标记为通过。需要使用当前有效的 Tranquility EVE 应用回调完成一次新授权，并分别提供 Director/CEO 与受限站点角色后重跑本验收。
