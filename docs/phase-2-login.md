# EVE 身份登录交付记录

> 历史阶段 / 方案记录：保留当时决策和测试结果，本文不是当前能力清单或最新部署指令。当前状态见[项目状态](project-status.md)，尚未完成事项见[待办](backlog.md)。

> 历史交付记录：以下范围、迁移版本与测试结果对应当时阶段。当前状态见[项目状态](project-status.md)，当前运行步骤见[开发指南](development.md)，后续变化见[变更记录](../CHANGELOG.md)。

这是最初身份登录阶段的历史记录。后续已配置本机凭据并实现职务授权；当前状态见 [军团权限指南](integrations/seat-authorization.zh-CN.md)，不再按本文的空 scope 范围运行。

日期：2026-09-14。本次按“先做 ESI 登录”完成 EVE SSO 身份登录代码，尚无真实开发者应用凭据。

## 已完成

- 新增 identity 与 eve 后端模块，eve 显式依赖 identity；宿主注入身份服务和会话鉴权。
- 官方发现文档与 JWKS 缓存、保密客户端授权码流程和 PKCE S256；一次性 state 同时绑定浏览器与原会话，回调原子消费。
- JWT RS256、issuer、双 audience、时间、角色/owner 及可选 azp/tenant 检查。
- 本站账号、角色归属、会话轮换、到期、退出、CSRF 防护、角色转让隔离；没有自动授予军团或管理员权限。
- `/login` 页面、顶部图标入口、未配置提示、官方登录按钮、角色显示和退出；统一页面注册、错误处理、键盘焦点和移动端布局。
- 默认模块配置、OpenAPI，以及中英文登录配置指南。

## 验证

| 检查 | 结果 |
| --- | --- |
| Go vet / tests | 通过 |
| 真实 PostgreSQL 16.14 集成测试 | 通过；迁移生命周期、并发首次登录、会话轮换/到期、归属变化撤权、Origin/CSRF 拒绝、浏览器绑定及回调防重放 |
| JWT 客户端测试 | 通过；测试 RSA 签名、错误算法/签名/声明拒绝、PKCE 和 Basic 请求、非可信发现地址拒绝 |
| sqlc 再生成 | 前后模块 Go 文件哈希一致 |
| 前端 lint / TypeScript / 构建 | 通过，lint 无警告 |
| Vitest | 11 项通过 |
| Playwright | 14 项通过，单 worker，桌面/手机；包括登录状态、表单提交/取消重试、CSRF 退出及原有工作台回归 |
| 截图 | 已查看 1440px 已配置和 375px 未配置登录页；配置存在状态使用测试响应，非真实凭据 |

本机开发库已执行 `00002_identity.sql`、`00003_eve_login.sql`，Goose 版本为 3；foundation marker 仍为 1。旧认证草稿保持隔离。真实 EVE 客户端凭据没有生成或伪造。

## 待真实联调与范围

用户尚未创建开发者应用；真实 CCP 授权界面、所注册应用的 PKCE 配合、真实角色回调仍待凭据配置后验证。测试提供方和测试签名不等于真实联调通过。

本次不请求私有 ESI scope，也不持久化 EVE token；长期令牌加密与刷新/撤销、QQ/KOOK 资料补全门禁、完整军团 RBAC、角色关联、机器人和游戏数据同步待后续实施。Cookie/Origin、直接对端 IP 限流及生产代理约束见 [中文配置](integrations/eve-login.zh-CN.md) / [English setup](integrations/eve-login.en.md)。未验证 Docker、PostgreSQL 18、Linux 运行、远程 CI 或生产负载。
