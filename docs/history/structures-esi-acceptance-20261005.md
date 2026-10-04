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

## 生产 CEO 复验（2026-10-05）

使用生产数据库中当前有效的 CEO 角色 `Nuter Zero`（角色 ID 仅用于内部验收，不写入文档）和现有 Tranquility 授权完成真实读取：

- Upwell 列表返回 20 座建筑，20 座均带 `fuel_expires`；POS 列表返回 13 座。
- 13 个 POS 详情全部成功，合计取得 13 条燃料明细（类型 ID 与数量）；修复了 ESI 详情所需的 `system_id`、官方 `fuels` 字段及数值型 `type_id` 映射。
- 重复读取命中私有缓存并保持观测时间；将目标军团改为错误 ID 被后端拒绝。
- 生产应用版本为 `v0.1.0-structures-pos-fuel-json-20261005`，应用 `active/ready`，匿名建筑接口 401。

受限站点角色、撤权/过期授权和陈旧缓存失败场景仍未完成；本次 CEO 证据不代表这些边界已通过。
