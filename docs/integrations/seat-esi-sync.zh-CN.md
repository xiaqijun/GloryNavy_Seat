# SeAT ESI 同步调研与本项目建议

> 接入基线 / 历史调研：本文包含当时建议，不等于全部已实现。当前模块指南见[接入目录](README.md)，当前版本与待办见[项目状态](../project-status.md) / [待办](../backlog.md)。上游核验日期保留原记录。

核验日期：2026-09-14。状态：**调研与建议，未实施通用同步框架**。[English](seat-esi-sync.en.md)

## 1. 结论与范围

SeAT 将 ESI 数据采集组织为定时调度、实体批次、资源任务、统一客户端和数据库模型。值得借鉴的是任务拆分、错峰调度、授权检查和失败隔离；本项目继续采用 Go 模块化单体，按已记录的 River + PostgreSQL 方向实现持久化任务。

本次阅读 SeAT 官方文档及 `eveseat/eveapi` 固定提交 `990a0a29649d0701f35605739c2e9b823275a13a`，提交时间为 2026-08-09。GitHub 最新发布查询返回 5.0.37；本文的具体行为以固定提交为准，不将分支快照等同于该发布标签，也不代表所有部署实例。未启动 SeAT，未做真实 ESI 同步或吞吐测试。

## 2. SeAT 的处理链

```mermaid
flowchart LR
    A[调度器 / 手动更新] --> B[选择到期角色与军团]
    B --> C[Character / Corporation 批次]
    C --> D[Redis 队列]
    D --> E[Horizon 管理的 Worker]
    E --> F[任务中间件]
    F --> G[EsiClient / Eseye]
    G --> H[ESI]
    H --> I[资源任务映射并写入模型]
    I --> J[页面查询本地数据]
```

- `eveapi` 包包含当前的调度命令、批次、资源任务、模型及 ESI 适配器。不要因旧文档引用 `eveseat/console` 就把旧 console 包当作当前实现入口。[包依赖](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/composer.json)
- Redis 承载队列，Horizon 负责 worker 管理与监控；主要 ESI 队列为 `characters`、`corporations`、`public`。`high` 名称本身不产生严格优先级。[官方队列说明](https://eveseat.github.io/docs/developer_guides/job_queue_flow/)
- 业务任务依赖 `EsiClient` 契约，默认由 `EseyeClient` 实现；响应缓存使用独立 `eseye` store，其默认驱动为 **file**，可配置 Redis。不能把队列、响应缓存、数据库快照混为同一层。[服务绑定](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/EseyeServiceProvider.php)、[客户端](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Services/EseyeClient.php)、[缓存配置](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Config/eseye-cache.php)

## 3. 什么时候同步

SeAT 把角色令牌分配到调度 bucket，分散整批同步负载。`BucketManager` 使用 120 秒平均批次时长、3600 秒更新窗口计算容量，形成最多约 30 个桶；这些是实现参数，不是 ESI 协议限制。[BucketManager](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Traits/BucketManager.php)

默认种子计划每两分钟执行 `seat:buckets:update`。每次选下一桶，按角色 `update_interval` 判断是否到期，批次间隔下限一小时。`last_update` 在派发前写入，表达“最近调度”，不代表所有数据已同步成功；较长周期角色另有 token keep-alive 分支。[调度命令](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Commands/Seat/Buckets/Update.php)

通知、合同、击杀等另有周期命令，部分计划随机偏移以减少请求集中。源码中的默认计划不等于管理员当前配置，更不能理解为所有资源每小时必然更新一次。[ScheduleSeeder](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/database/seeders/ScheduleSeeder.php)

**项目建议：**按“资源类型 + 实体 ID”保存 `next_due_at`，结合 ESI 缓存到期、退避和随机偏移选取下次执行时间。保留错峰思想，不照搬固定 30 桶或一小时下限。手动刷新也只派发到期任务，不能绕过缓存或限流。

## 4. 任务、权限与多角色

`Character` 和 `Corporation` 组合具体任务；`Bus::addAuthenticatedJob` 先按已授予 scope 筛选。每类资源任务声明 endpoint、scope、兼容日期，并自行完成映射与持久化。批次使用 Laravel batch 包装任务链，不能简单理解为同一角色的全部资源同时并发。批次有成功、失败和结束回调；角色的 `CharacterBatchProcessed` 在 `finally` 发出，事件不等于全部成功。[Bus](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Bus/Bus.php)、[角色批次](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Bus/Character.php)、[军团批次](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Bus/Corporation.php)

自动 bucket 更新从已有职务记录为 Director 的角色发起军团同步，并在**本次命令执行内**按军团去重；这不等于跨进程、跨桶的持久化唯一任务约束。具体军团任务还检查 scope、token 版本、NPC 军团和所需职务；`RequireCorporationRole` 使用普通 `roles`，把 Director 加入可接受职务。[自动派发](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Commands/Seat/Buckets/Update.php)、[军团任务基类](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Jobs/AbstractAuthCorporationJob.php)、[职务检查](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Jobs/Middleware/RequireCorporationRole.php)

**项目建议：**

- 每个绑定角色独立保存凭据、scope、归属和同步状态。本站主角色不自动成为军团数据来源。
- 军团任务唯一键以军团和资源为核心，来源角色另外记录。按具体端点条件选合格凭据，执行前复核归属和职务；失效后重新选择合格来源，不能借切换角色规避上游限制。
- ESI scope、游戏职务、本站访问权限分别检查。同步成功不自动授予成员查看权限。
- 任务定义由已启用模块显式注册；新业务自己拥有落库逻辑和私有 store。不要在通用同步层硬编码所有未来业务。

## 5. 令牌、缓存、限流和错误

SeAT 的 `InteractsWithToken` 将客户端刷新后的 access token、refresh token 和到期时间回存。`EsiBase` 在请求异常分支也尝试保存刷新结果；明确的永久无效 refresh token 错误会转为永久失败并删除令牌。认证任务配置 `WithoutOverlapping` 锁，但不能仅凭传入角色 ID 就断言不同任务类共享同一刷新锁。[令牌回存](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/InteractsWithToken.php)、[EsiBase](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Jobs/EsiBase.php)

该提交的 `EsiBase` 配置最多 3 次未处理异常、按尝试次数增加的退避；`CheckEsiRateLimit` 使用共享错误计数，达到内部阈值 80 后延后 300 秒。这个计数策略不等同于 CCP 当前完整限流协议。`CheckEsiRouteStatus` 虽存在，但在默认中间件列表中被注释；服务器状态检查仍启用。[限流中间件](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Jobs/Middleware/CheckEsiRateLimit.php)、[EsiBase](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Jobs/EsiBase.php)

CCP 当前同时存在旧错误预算和逐步启用的分组滑动窗口：新限制按 group 与调用身份区分，应读取 `X-Ratelimit-*`、遵守 `Retry-After`，同时保留旧错误头与 420 处理；部分 429 可能没有完整限流头。SeAT 的调度 bucket 与 CCP 限流 bucket 是两回事。[CCP 限流文档](https://developers.eveonline.com/docs/services/esi/rate-limiting/)

缓存应遵守 `Expires`；适用 GET 保存 ETag，通过 `If-None-Match` 处理 304。分页应核对 `Last-Modified` 等一致性信息，防止翻页途中数据更新。请求携带应用身份及维护者联系方式。这些属于当前官方接入要求/建议，不声称上文 SeAT 快照完整实现了全部细节。[CCP 最佳实践](https://developers.eveonline.com/docs/services/esi/best-practices/)

本项目应明确错误分类：

| 结果 | 建议行为 |
| --- | --- |
| 网络超时、5xx | 有界退避与随机偏移；保留最近成功数据 |
| 420、429 | 根据作用域暂停/延后，遵守上游响应；多 worker 共享预算 |
| access token 到期 | 同角色协调刷新，防止 refresh token 轮换覆盖 |
| 确认 refresh token 撤销/无效 | 停止该凭据任务，标记重新授权，失效权限事实退出判断 |
| 缺 scope、缺职务、退出军团 | 阻塞对应资源并重算来源；不能把所有 403 一律认作整枚令牌撤销 |
| 304 | 本次检查成功但内容未变化，分别记录检查时间与内容版本 |

## 6. 数据怎么写入

SeAT 按资源选择更新方法：职务任务新增当前职务、删除不再存在的职务；资产任务逐页 upsert，全部页完成后才清除旧条目；钱包流水按记录 ID 去重，向后遍历到已知记录或末页。不同资源没有统一的“清空再导入”流程。[职务](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Jobs/Character/Roles.php)、[资产](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Jobs/Assets/Character/Assets.php)、[流水](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Jobs/Wallet/Character/Journal.php)

项目建议区分快照与历史记录。完整快照通过批次版本或暂存区校验后发布；部分分页失败不能删除旧数据或声称全量成功。历史记录使用业务唯一键与可重入消费，重试不会重复计数。将 `last_attempt_at`、`last_success_at`、`next_due_at`、内容时间、错误类别、来源角色、批次 ID 分开记录，为以后报表提供数据时间与完整性依据。

SDE 仍为独立静态数据版本导入流程。SeAT 有单独 SDE 调度命令；本次没有审计其完整导入实现。我们的 ESI 动态数据任务可引用已发布的 SDE 版本，但不应为每个角色重复导入 SDE。

## 7. 现有实现与下一阶段建议

| 能力 | 本项目现状 | 建议下一步 |
| --- | --- | --- |
| EVE 登录、独立角色令牌 | 已实现，加密落库 | 复用并抽出统一凭据协调接口 |
| 职务与归属同步 | 专用 worker，数据库到期时间及行锁领取 | 迁入统一持久化任务，避免旧新 worker 重复工作 |
| 请求控制 | 公共响应内存缓存、进程内统一暂停、基础重试 | 资源缓存、ETag、按组共享限流与错误分类 |
| 通用任务 | River 仅为技术规划，尚未接入 | 注册、去重、延后、重试、崩溃恢复、优雅退出 |
| 业务同步状态 | 授权/职务相关状态 | 按资源记录最近成功、下一次执行和失败原因 |
| 资产、钱包、报表等 | 对应功能未开发 | 随业务接入任务、数据模型和权限，不提前开放配置 |

现状依据：[authorization.go](../../internal/modules/eve/authorization.go)、[esi.go](../../internal/modules/eve/esi.go)、[架构](../architecture.md)。当前 worker 在事务内持有角色锁进行网络调用，适合现有窄流程；通用任务设计应将任务领取、凭据刷新协调、业务提交的锁边界明确区分，避免长事务随着业务数量增长。

建议第一批交付：**统一同步框架 + 现有角色归属/职务/军团基础信息迁移 + 同步状态及受控重试入口**。取得 57 项 scope 不等于立即抓取所有资源；同步管理权限只有在对应功能完成时才加入可配置目录。

实施验收至少覆盖：重复入队、双 worker 领取、进程中断恢复、轮换令牌并发、刷新成功后 ESI 失败、缺 scope/职务、军团变更、缓存命中与 304、420/429 延后、部分分页失败及敏感信息脱敏。本次仅完成调研文档，未新增 API、队列、迁移或页面。
