# 军团职务与权限交付记录

> 历史阶段 / 方案记录：保留当时决策和测试结果，本文不是当前能力清单或最新部署指令。当前状态见[项目状态](project-status.md)，尚未完成事项见[待办](backlog.md)。

> 历史交付记录：以下范围、迁移版本与测试结果对应当时阶段。当前状态见[项目状态](project-status.md)，当前运行步骤见[开发指南](development.md)，后续变化见[变更记录](../CHANGELOG.md)。

日期：2026-09-14。用户要求“先按 SeAT 中的实现”，本轮落地权限基础与职务数据链路。

- EVE 新增本人军团职务 scope，AES-256-GCM 凭据保存、刷新、按缓存周期同步、撤销与过期处理。
- access 模块提供 CEO/Director/游戏职务映射、独立本站角色、军团/联盟过滤器、管理 API 与审计；管理员只能通过运维明确指定。
- 登录页增加军团、职务、地点职务折叠、同步状态和重新授权操作。
- 新增两份迁移，本机 Goose 到 5，基础结构 marker 保持 1。本机已生成令牌密钥；没有自动给任何账号管理员权限。
- 保留 QQ/KOOK 独立身份确认边界。Squads、权限管理页面、小号关联和全军团成员职务采集尚未实现。

验证：Go vet/测试含独立 PostgreSQL schema 集成测试通过，覆盖跨军团访问、职务映射、手动过滤器、撤权、加密身份绑定、并发刷新、刷新后 ESI 失败恢复、缓存及限流、换团与重复登录缓存保护。前端 lint 无警告，13 项单元测试与生产构建通过，16 项桌面/手机浏览器测试通过。另检查 1440px 与 375px 的模拟授权页面，无横向溢出，图标操作至少 44px。OpenAPI YAML 语法检查通过。

验证环境为本机 PostgreSQL 16、Go 1.27.1、Chrome；没有在本轮执行远端 CI、Linux race 或目标 PostgreSQL 18 测试。真实 CCP 回调和 ESI 职务数据尚未联调，需在现有 EVE 应用登记新增 scope 后由玩家授权。完整配置见 [中文](integrations/seat-authorization.zh-CN.md) / [English](integrations/seat-authorization.en.md)。
