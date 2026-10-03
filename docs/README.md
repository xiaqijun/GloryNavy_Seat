# 项目文档

接入方案与阶段状态：[EVE Sentry 本站接入](plans/eve-sentry-integration.zh-CN.md)（SENTRY-01，密钥、监控奖励和收费配置已生产，真实证据与实账验收待完成）；[军团贷款与个人贷款](plans/loans.zh-CN.md)（LOAN-01，按用户要求暂缓实施）。

GloryNavy 是面向 EVE Online 国际服 Tranquility 的自研军团管理平台。更新：2026-09-23。

## 先看这里

| 目的 | 入口 |
| --- | --- |
| 当前生产版本、已上线功能和验证边界 | [项目状态](project-status.md) |
| 尚未完成、需联调、需核实与暂停的事项 | [待办清单](backlog.md) |
| 本地启动、配置、迁移与调试 | [开发指南](development.md) |
| 发布、备份、反代、证书与恢复 | [生产部署](deployment.md) |
| 已采用技术及可选组件 | [技术栈](tech-stack.md) |
| 模块边界与新增模块 | [架构](architecture.md)、[模块开发](module-development.md) |
| 行为与 API | [OpenAPI](../api/openapi.yaml)、[中英文接入目录](integrations/README.md) |
| 文档和交付维护 | [贡献约定](../CONTRIBUTING.md)、[变更记录](../CHANGELOG.md) |

## 功能指南

| 功能 | 文档 |
| --- | --- |
| 公开军团首页与数据 | [中文](integrations/public-corporation.zh-CN.md) / [English](integrations/public-corporation.en.md) · [页面](ui/homepage.md) |
| 登录与本站会话 | [中文](integrations/eve-login.zh-CN.md) / [English](integrations/eve-login.en.md) · [页面](ui/login.md) |
| 默认登录 scopes | [中文](integrations/seat-login-scopes.zh-CN.md) / [English](integrations/seat-login-scopes.en.md) |
| 多角色参考与规则 | [中文](integrations/seat-multi-character.zh-CN.md) / [English](integrations/seat-multi-character.en.md) · [页面](ui/account.md) |
| 账号合并 | [中文](integrations/account-merge.zh-CN.md) / [English](integrations/account-merge.en.md) |
| QQ / KOOK 资料 | [中文](integrations/community-profile.zh-CN.md) / [English](integrations/community-profile.en.md) |
| 本站权限与成员范围 | [中文](integrations/seat-authorization.zh-CN.md) / [English](integrations/seat-authorization.en.md) · [页面](ui/access.md) |
| ESI 后台同步 | [中文](integrations/esi-sync.zh-CN.md) / [English](integrations/esi-sync.en.md) · [页面](ui/sync.md) |
| ESI 客户端与令牌观测 | [中文](integrations/esi-client.zh-CN.md) / [English](integrations/esi-client.en.md) |
| 限流窗口与预算 | [中文](integrations/esi-rate-limits.zh-CN.md) / [English](integrations/esi-rate-limits.en.md) |
| SDE 名称与静态参考 | [中文](integrations/sde-names.zh-CN.md) / [English](integrations/sde-names.en.md) |
| 官方游戏术语 | [中文](integrations/eve-terminology.zh-CN.md) / [English](integrations/eve-terminology.en.md) |
| 个人 / 军团合同 | [中文](integrations/contracts.zh-CN.md) / [English](integrations/contracts.en.md) · [页面](ui/contracts.md) |
| 个人 / 军团钱包 | [中文](integrations/wallet.zh-CN.md) / [English](integrations/wallet.en.md) · [页面](ui/wallet.md) |
| 吉他估价与合同估价 | [中文](integrations/market.zh-CN.md) / [English](integrations/market.en.md) · [页面](ui/market.md) |
| 活动、在线与军团 PAP | [中文](integrations/attendance.zh-CN.md) / [English](integrations/attendance.en.md) · [页面](ui/attendance.md) |
| 联盟 PAP 服务同步 | [中文](integrations/winterco-pap.zh-CN.md) / [English](integrations/winterco-pap.en.md) |
| 配装库与游戏保存 | [中文](integrations/fittings.zh-CN.md) / [English](integrations/fittings.en.md) · [页面](ui/fittings.md) |
| 技能与达标检查 | [中文](integrations/skills.zh-CN.md) / [English](integrations/skills.en.md) · [页面](ui/skills.md) |
| 舰船损失 | [中文](integrations/character-losses.zh-CN.md) / [English](integrations/character-losses.en.md) |
| 福利与自动合同核验 | [中文](integrations/welfare.zh-CN.md) / [English](integrations/welfare.en.md) · [页面](ui/welfare.md) |
| 果壳币、奖励库与兑换 | [中文](integrations/exchange.zh-CN.md) / [English](integrations/exchange.en.md) · [页面](ui/exchange.md) |
| 审批中心 | [中文](integrations/approval.zh-CN.md) / [English](integrations/approval.en.md) · [页面](ui/approval.md) |
| 工作台与系统页 | [页面](ui/system.md) |
| 军团运营面板 | [页面](ui/operations.md) |
| 管理员成员资料 | [页面](ui/members.md) |

已上线功能的真实游戏验证仍需单独记录。联盟 PAP 自动兑换代码已发布，生产当前保持手动模式；不能将“定时同步”理解为“自动发币”。

## UI 与来源

- [通用设计规则](ui-design-rules.md)：Corporate Clean 强制提示词的项目适配；具体排版在页面说明中维护。
- [中英文界面](ui/language.md) / [English](ui/language.en.md)：固定文案、日期与数据名称边界。
- [第三方声明](third-party-notices.md)、[图片来源](../web/public/images/README.md)：许可证与资源出处，不能作为无用文档清理。
- [全站卡片检查](ui/card-space-review.md)、[排版回归](ui/layout-review-2026-09-20.md)、[图表检查](ui/chart-review-2026-09-22.md)、[术语审查](ui/game-terminology-audit-2026-09-20.md)：保留带日期的验收证据，当前行为以页面指南为准。
- [数据库性能](database-performance.md)、[生产耗时原始检查](reviews/production-query-performance-2026-09-22.md)。

## 历史、调研与方案

[历史目录与清理说明](history/README.md) / [截至 2026-09-23 的交付记录](history/project-status-through-2026-09-23.md)。阶段验收、早期技术调研、PAP 草案和配装排版调研均已集中到 `docs/history/`，只作为来源和决策证据；历史“未发布”“待联调”不直接作为当前待办。

当前仍需查阅的方案仅保留在 `docs/plans/`，集成行为以 `docs/integrations/` 为准。

## 文档职责

`project-status.md` 只维护当前结论；`backlog.md` 集中维护未完成事项及验收条件；历史发布证据进入 `history/`。模块运行与权限契约维护在 `integrations/` 双语指南；UI 规则和页面适配分开。方案、来源和历史测试不伪装成当前实现或本轮验证。

新增或删除文档要更新入口及相对链接。官方资料的核验日期按实际调研保留，整理文档不等于重新查证上游 API。依赖与配置分别以仓库锁文件、环境示例和运行指南为准。
