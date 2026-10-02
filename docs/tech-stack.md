# 技术栈

更新：2026-09-23。以仓库依赖、实现和最近部署记录为依据，不把建议组件写成已接入。当前能力见[项目状态](project-status.md)，待办见[清单](backlog.md)。

## 已采用

| 层 | 当前实现 | 依据 / 边界 |
| --- | --- | --- |
| 后端 | Go 1.27.1、net/http + chi v5 | 版本以 `go.mod` 为准，模块化单体，显式注册 |
| 数据库 | PostgreSQL；生产 18.6，本机独立开发实例 16 | 本地与生产分库，镜像/端口见部署与开发指南 |
| SQL | pgx v5 + sqlc | 无 ORM；每模块私有 SQL/store，跨模块经宿主服务注入 |
| 迁移 | Goose + River 独立迁移线 | `npm run db:migrate` 处理两条线；当前生产 Goose 46 |
| 后台任务 | River 0.47.0 + PostgreSQL | 角色同步、业务核验等共用宿主持久队列；幂等、租约与发布 fence |
| 前端 | React 19.2.8、TypeScript 6.0.2、Vite 8.3.0 | CSR 静态产物；Node >=24.15.0 用于开发/构建 |
| 路由与请求 | React Router 7.18.3、TanStack Query 5.102.8 | 页面懒加载、查询缓存与失效；URL 保存主要筛选 |
| UI | Tailwind CSS 4.3.3、Radix、项目内 shadcn 风格组件、Lucide | 后台遵循 Corporate Clean 及[项目规则](ui-design-rules.md) |
| 图表 | Apache ECharts 6.1.0、共享主题 | 已用于 PAP、钱包、在线、限流等页面，按需加载 |
| 首页动效 | GSAP 3.15.0 + @gsap/react 2.1.2 / ScrollTrigger | 仅公开首页，减少动态效果与移动端降级；不扩展为后台全站动效 |
| 身份 | EVE SSO、本站数据库会话、HttpOnly Cookie | EVE token 后端加密保存；会话续期、CSRF 与对象权限独立 |
| 国际化 | 项目 `msg` / 英文词表、服务端 locale、StaticDataService | 玩家内容和协议枚举保留原文，官方术语/SDE 名称分开维护 |
| 检查 | Go testing/httptest、PostgreSQL 集成测试、Vitest、Playwright、oxlint | 验证范围按改动选择；模拟通过不等于真实游戏联调 |
| 生产入口 | 1Panel OpenResty + HTTPS / ZeroTier | 静态前端与 API 同源；Go 由 systemd 管理，PostgreSQL 独立容器 |

依赖精确版本以 `go.mod`、`web/package-lock.json`、容器配置为准。表中版本是本次仓库核对结果，不是对上游最新版本的声明。

## 数据与模块

已编译模块：system、identity、eve、access、community、attendance、exchange、fittings、skills、wallet、market、welfare、approval、sentry。启用清单由 `MODULES` 显式配置，不等于所有模块在每个环境都已启用。未设置时采用 `internal/config/config.go` 的默认值；示例配置可能启用更多模块，保留原环境清单后按依赖追加。sentry 密钥申请第一阶段已完成本地两端 HTTP 与 PostgreSQL 联调，生产凭据和部署仍待执行。

ESI 统一处理令牌刷新、缓存、共享限流、超时与同步任务。上游响应缓存到期才重新抓取；页面优先读本地快照，不逐项请求 ESI。SDE 数据库保存物品/星系名称，配装前置技能、技能类别与术语还有固定构建的参考数据，不宣称已导入全量 Dogma。详见[ESI](integrations/esi-client.zh-CN.md)、[SDE](integrations/sde-names.zh-CN.md)。

金额以精确数值处理，API 使用十进制字符串；游戏 ID 与版本也使用字符串传输。时间存 UTC，业务统计的月份/时区按各模块契约，不用界面语言改变统计边界。查询、导出、审批、发放始终做后端对象权限检查。

## 部署与观测

`浏览器 → OpenResty（HTTPS、静态文件）→ ZeroTier → Go API / River → PostgreSQL`

生产不运行 Vite/Node 构建进程。发布包含构建、校验、数据库/配置备份、两条迁移线检查、兼容切换与回归，详见[生产部署](deployment.md)。Caddy 是早期建议，当前生产未采用；不要据旧调研重建入口。

Go slog、请求 ID、River 状态、ESI 观测和应用慢查询日志已具备；`DB_SLOW_QUERY_MS` 默认 500。数据库累计统计 `pg_stat_statements` 未启用，外部告警与恢复演练见[待办](backlog.md)。发布前备份不等于已建立定期异机灾备。

服务器配置仅用于部署预算与实际压测，不反复作为功能选择限制。连接池、缓存、任务并发应以真实负载和当前配置为准，不把早期建议容量当成验证值。不依赖 Redis、微服务、Kubernetes 或独立 BI 平台。

## 已实现与可选扩展

- 已有页面聚合/图表与合同 CSV 导出。通用报表平台、异步导出中心、XLSX 尚未交付，按具体指标与权限需求设计。
- TanStack Table、React Hook Form、Zod、Excelize 仍是按需候选，当前依赖未引入；不要把它们写成现有实现。
- 舰船模拟页面已按用户要求撤下。历史 `@eveshipfit/dogma-engine` / `@eveshipfit/sde` 依赖与兼容代码仍在仓库，当前页面不加载模拟；删除前单独分析兼容范围。
- QQ/KOOK 手填与完整度门禁已实现，后续机器人只经业务适配器确认身份；平台 OAuth 不是现有需求。
- 联盟 PAP 当前只支持管理员手动兑换，军团 PAP 支持手动/自动。联盟自动兑换是明确待做需求，不能混称已完成。

架构边界见[模块架构](architecture.md)和[开发约定](module-development.md)。初始选型依据保留在[数据库调研](database-access-research-2026-09-13.md)和[SeAT 调研](seat-research-2026-09-13.md)，不以历史建议覆盖当前实现。
