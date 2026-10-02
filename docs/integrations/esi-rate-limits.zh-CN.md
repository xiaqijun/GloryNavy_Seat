# ESI 限流官方规范核对

> 状态索引（2026-09-23）：本指南对应的业务已上线；未交付的扩展与真实验收缺口见[待办](../backlog.md)，当前版本见[项目状态](../project-status.md)。下文带日期/Goose 的发布前记录保留当时语境，不能据此认定当前未发布；升级执行仓库最新迁移，不停在模块初始版本。

核对日期：2026-09-14。对应 [English](esi-rate-limits.en.md)。本文保留调研结论，末节跟进 2026-09-15 的实现适配；生产验证以项目状态为准。

## 来源与版本

- [项目兼容日期 OpenAPI](https://esi.evetech.net/meta/openapi.json?compatibility_date=2026-08-18)：本次实际下载并遍历 method/path 的 `x-rate-limit`。
- [官方限流文档](https://developers.eveonline.com/docs/services/esi/rate-limiting/)：组、身份、滑动窗口、单次计费与响应头。
- [官方最佳实践](https://developers.eveonline.com/docs/services/esi/best-practices/)：固定错误窗口与缓存。
- [兼容日期说明](https://developers.eveonline.com/docs/services/esi/overview/)：省略日期会采用最早可用版本。本次无日期 OpenAPI 的 info.version 为 2020-01-01，不能称作最新版；项目指定日期返回 2026-08-18。

## 本次核对结果

指定日期规范有 233 个 method/path 操作，其中 171 个声明新令牌桶、62 个未声明；46 个组的 window-size **全部为 15m**，容量不同。这是本次快照，不承诺未来相同，也不把 15 分钟硬编码进客户端。

| 接口用途 | group | 容量 | 滑动窗口 |
| --- | --- | ---: | --- |
| 个人合同列表、物品、出价 | char-contract | 600 | 15m |
| 军团合同列表、物品、出价 | corp-contract | 600 | 15m |
| 本人军团职务 | char-detail | 600 | 15m |
| 军团成员职务列表 | corp-member | 300 | 15m |
| 角色位置、在线、当前舰船 | char-location | 1200 | 15m |
| 舰队相关接口 | fleet | 1800 | 15m |
| 区域市场订单 | market-order | 12000 | 15m |
| 角色通知 | char-notification | 15 | 15m |

POST /characters/affiliation、GET /characters/{character_id}、GET /corporations/{corporation_id} 均未声明 x-rate-limit。应保留“无新桶声明”这一事实，不制造名为“未识别分组”的真实桶。某次请求没有响应头（如本地缓存或网络失败）也不等于已证明接口没有桶；接口声明与运行时观测缺失须分开。

同组与同调用身份共享一个桶，不能按列表/物品/出价拆成三个 600 配额。认证身份按 applicationID:characterID；匿名按出口身份。滑动窗口按各次消耗时间逐步返还，不是等待最后请求后 15 分钟才整体补满。文档状态规则为 2xx/3xx/4xx/5xx 对应 2/1/5/0，429 除外；实际单次计量读取 X-Ratelimit-Used。

旧错误限流使用固定窗口及 X-ESI-Error-Limit-Remain/Reset；不与新令牌桶混算。官方还说明某些游戏服务内部限流会返回不带新桶头的 429，因此“没有新桶”不能写成“无限制”。

缓存 TTL 是另一条时钟：个人合同列表与出价为 300 秒，物品为 3600 秒；军团合同列表为 300 秒，物品/出价为 3600 秒。它们的令牌桶窗口仍为 15m。本地新鲜缓存不发 ESI 请求；条件请求得到 304 仍消耗对应配额。

## 实现跟进（2026-09-15）

调研发现的占位桶、重复预扣和静默恢复问题已在 Goose 20 改造中处理：嵌入兼容日期目录，只创建真实声明/观测分组；并发预留后按实际 Used 结算，按各笔时间恢复，缺失计量保守留额。Remaining 只收紧本地预算，不能把本地估算称作 CCP 实时余额。Retry-After 不再被强制拉长到 60 秒。

页面增加容量/窗口来源、下次配额恢复和接口消耗对比，基础资料没有新桶时不显示虚构桶。实现、更新目录、缓存边界及升级/回退步骤见[客户端指南](esi-client.zh-CN.md)。
