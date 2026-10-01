# 首页公开军团资料

> 状态索引（2026-09-23）：本指南对应的业务已上线；未交付的扩展与真实验收缺口见[待办](../backlog.md)，当前版本见[项目状态](../project-status.md)。下文带日期/Goose 的发布前记录保留当时语境，不能据此认定当前未发布；升级执行仓库最新迁移，不停在模块初始版本。

2026-09-23 已发布生产，后端 v0.1.0-public-home-20260923；验收见[项目状态](../project-status.md)。[English](public-corporation.en.md)

## 范围与契约

匿名 GET `/api/v1/eve/public/corporation` 固定查询 Glory Navy（98530802）。无查询参数，无可指定目标、路径或上游地址，不接受成员凭据。200 返回 `data`：

- corporation_id、name、ticker、member_count、date_founded：官方军团资料，member_count 是游戏角色数。
- alliance（可省略）：id、name、ticker、corporation_count（无法读取数量时 null）。数量属于整个联盟，不能当作本军团规模。
- updated_at：军团资料的 ESI 验证时间（回退内容更新时间）；stale 表示本次刷新失败、正在返回此前成功快照。

初次失败返回 503/public_profile_unavailable；不暴露上游错误详情。数据缺失显示“—”，不伪造 0。前端仅读取公开投影和现有会话入口；不匿名加载模块目录、成员明细、PAP、审批、钱包、资产或 scope 配置。首版战绩仅有公开外链，后续本地新增活动聚合如下。

## 击毁与在线角色（2026-09-23，已发布 public-strength-r1）

用户明确要求首页展示击毁与在线数据，多开不合并。新增匿名 GET `/api/v1/eve/public/activity`，固定军团 98530802，无可指定目标或上游地址。返回 `corporation_id`、`combat`、`online`；两个来源独立失败为 null，不影响军团资料和招募。

- combat：当前 UTC 月及此前五个月的 `months[{month,kills,value}]`、`updated_at`、`stale`。固定读取 [zKillboard 统计 API](https://zkillboard.com/api/docs/) `/api/stats/corporationID/98530802/kills/`，统计军团参与的公开击毁，价值为 zKillboard ISK 估算，不是钱包收入或仅最终一击。缺月为 null；只有明确损失侧且无击毁侧的月份按该接口稀疏格式为 0。上游其他身份字段、列表和原始响应不透传。10 分钟进程缓存、并发合并、8 秒超时、2 MiB 限制、固定项目 User-Agent、自动 gzip、拒绝重定向；失败退避一分钟，保留原快照及时间并标 stale。无访客请求时不抓取，重启后重新获取。
- online：`characters`（有效采样在线角色数）、`covered_characters`、`bound_characters`、`updated_at`、`expires_at`。每个角色分别计数，多开不按账号合并。仅当前有效绑定且军团资料有效的 Glory Navy 角色；校验 owner hash、授权代次、授权状态、`esi-location.read_online.v1` 和 180 秒内的本军团采样。无有效采样时 characters=null，不能当作全员离线。覆盖数量表示已采样角色/已绑定军团角色，不代表全军团覆盖。
- 在线数据经宿主注入 identity 绑定和 EVE 私有查询聚合，只读取已有 River 在线采样；不因匿名访问请求角色 ESI。30 秒服务端缓存，失败不保留旧在线数；整个 API 为 no-store，浏览器每 30 秒检查，到 expires_at 隐去旧数。公开仅总数，不返回角色/账号 ID、姓名、位置或个人状态。本轮无需新增 scope、配置或迁移。

公开聚合是用户明确批准的局部例外，不改变其他成员数据 API 的鉴权。后端与前端一同发布；旧后端返回 404 时，新界面仅该区域不可用。

最终界面只读取当前月数字，不展示趋势图、覆盖数量、统计口径或来源时间小字。六个月投影仍是 API 返回契约；采样与来源校验不会因为精简 UI 而取消。

在线缓存最长 30 秒；样本提前到期时，下次请求立即重新聚合。浏览器按 expires_at 提前检查（最短 1 秒），避免等满 30 秒产生空值间隔；始终不延长旧样本有效期。

## ESI 与缓存

由宿主注入共享 ESIService，依次使用官方匿名 GET：

1. `/corporations/98530802/`
2. 存在联盟时 `/alliances/{alliance_id}/`
3. `/alliances/{alliance_id}/corporations/`，计数即可，不对每个军团发请求。

参考：[官方 API Explorer](https://developers.eveonline.com/api-explorer#/operations/GetCorporationsCorporationId)。

不使用角色 token，不新增 SSO scope。复用现有客户端兼容日期、PostgreSQL 缓存、ETag/Expires 与共享限流；不另建抓取器。仅投影所列字段，不透传 description、CEO、税率或任何私有字段。

进程内保留聚合快照，刷新并发合并；按上游各项有效 Expires 的最早时间刷新，缺失时间及失败退避一分钟。共享刷新有 12 秒预算，单个访客离页不会中断其他访客的刷新。军团刷新失败保留原值和时间并标 stale；联盟读取失败仅缺失联盟资料/数量。聚合快照不单独落库，服务重启后从共享 ESI 缓存重新构建；重建失败且没有内存快照时仍返回 503。

浏览器每 5 分钟检查一次，网络更新仍遵循上游缓存；HTTP 聚合结果缓存 60 秒，初次错误响应 30 秒。没有独立 River 任务。既有 ESI 服务不可用时首页介绍正常，公开数据区降级。

## 发布与验证

前后端一起发布；无迁移、环境变量或会话变更。回退前端可以忽略新端点，旧后端配新前端只会导致公开数据不可用。

本轮已验证字段白名单、无角色 token、固定目标、聚合缓存、失败退避、旧值保留和缺失不当作零。真实本地接口读取到官方公开资料；具体数量不是配置默认值，会随 ESI 更新。中英文、响应式及减少动态效果见[首页](../ui/homepage.md)。
