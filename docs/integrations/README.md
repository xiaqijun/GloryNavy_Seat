# 接入与业务指南 / Integration and business guides

文档导航整理：2026-09-23；各文档的上游资料核验日期保持原记录。当前部署见[项目状态](../project-status.md)，未完成项见[待办](../backlog.md)。

Navigation updated 2026-09-23. This is not a new verification of upstream documentation. See the current project status and backlog; dated implementation notes do not establish today's release status.

## 当前指南 / Current guides

| 范围 / Scope | 语言 / Language |
| --- | --- |
| 公开军团首页与数据 | [中文](public-corporation.zh-CN.md) / [English](public-corporation.en.md) |
| 登录与本站会话 | [中文](eve-login.zh-CN.md) / [English](eve-login.en.md) |
| 默认登录 scopes | [中文](seat-login-scopes.zh-CN.md) / [English](seat-login-scopes.en.md) |
| 多角色参考与规则 | [中文](seat-multi-character.zh-CN.md) / [English](seat-multi-character.en.md) |
| 账号合并 | [中文](account-merge.zh-CN.md) / [English](account-merge.en.md) |
| QQ / KOOK 资料 | [中文](community-profile.zh-CN.md) / [English](community-profile.en.md) |
| 本站权限与成员范围 | [中文](seat-authorization.zh-CN.md) / [English](seat-authorization.en.md) |
| ESI 后台同步 | [中文](esi-sync.zh-CN.md) / [English](esi-sync.en.md) |
| 建筑管理 / Structure management | [中文](structures.zh-CN.md) / [English](structures.en.md) |
| ESI 客户端与令牌观测 | [中文](esi-client.zh-CN.md) / [English](esi-client.en.md) |
| 限流窗口与预算 | [中文](esi-rate-limits.zh-CN.md) / [English](esi-rate-limits.en.md) |
| SDE 名称与静态参考 | [中文](sde-names.zh-CN.md) / [English](sde-names.en.md) |
| 官方游戏术语 | [中文](eve-terminology.zh-CN.md) / [English](eve-terminology.en.md) |
| 个人 / 军团合同 | [中文](contracts.zh-CN.md) / [English](contracts.en.md) |
| 个人 / 军团钱包 | [中文](wallet.zh-CN.md) / [English](wallet.en.md) |
| 吉他估价与合同估价 | [中文](market.zh-CN.md) / [English](market.en.md) |
| 活动、在线与军团 PAP | [中文](attendance.zh-CN.md) / [English](attendance.en.md) |
| 联盟 PAP 服务同步 | [中文](winterco-pap.zh-CN.md) / [English](winterco-pap.en.md) |
| 配装库与游戏保存 | [中文](fittings.zh-CN.md) / [English](fittings.en.md) |
| 技能与达标检查 | [中文](skills.zh-CN.md) / [English](skills.en.md) |
| 舰船损失 | [中文](character-losses.zh-CN.md) / [English](character-losses.en.md) |
| 福利与自动合同核验 | [中文](welfare.zh-CN.md) / [English](welfare.en.md) |
| 果壳币、奖励库与兑换 | [中文](exchange.zh-CN.md) / [English](exchange.en.md) |
| 审批中心 | [中文](approval.zh-CN.md) / [English](approval.en.md) |
| 军团与个人贷款 | [中文](loan.zh-CN.md) / [English](loan.en.md) |
| EVE Sentry 预警平台密钥 | [中文](sentry-keys.zh-CN.md) / [English](sentry-keys.en.md) |

联盟 PAP 当前支持定时快照同步和管理员手动兑换，自动兑换待实现。EVE Sentry 密钥申请、远端密钥投影、时间授权/投递游标客户端、只读果壳币账单接口及受开关控制的 River 对账 worker 已在本站代码中实现；价格通过生产环境变量提供，预警扣费仍默认关闭，隔离生产账号币账验收未执行。合同自动核验已实现，但真实游戏交付验收与模拟测试分开记录。

Alliance PAP supports scheduled snapshot synchronization and administrator-triggered conversion; automatic conversion remains pending. EVE Sentry key requests, remote key projection, the time-grant/delivery cursor client, read-only Nutshell Coin usage endpoints, and the feature-gated River reconciliation worker are implemented in the Seat codebase; alert charging remains disabled pending production pricing, complete evidence, and live ledger acceptance. Automatic contract verification is implemented, but simulated tests do not establish live game fulfillment.

## 接入基线与历史调研 / Baselines and research

- ESI / SSO / SDE 接入基线：[中文](eve-integration.zh-CN.md) / [English](eve-integration.en.md)。包含规划内容，模块实现以以上专项指南为准。
- 官方来源核验：[来源记录](eve-sources.md)。保留实际核验日期，不将本轮文档整理记成外部资料重新验证。
- SeAT 同步调研：[中文](seat-esi-sync.zh-CN.md) / [English](seat-esi-sync.en.md)。
- 早期同步实施方案：[中文](esi-sync-plan.zh-CN.md) / [English](esi-sync-plan.en.md)，历史方案，不替代当前升级步骤。

中英文指南同步维护行为、权限、配置与兼容要求；历史部署证据集中到[交付历史](../history/project-status-through-2026-09-23.md)。
