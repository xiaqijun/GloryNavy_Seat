# SeAT 默认授权与独立登录页

> 状态索引（2026-09-23）：本指南对应的业务已上线；未交付的扩展与真实验收缺口见[待办](../backlog.md)，当前版本见[项目状态](../project-status.md)。下文带日期/Goose 的发布前记录保留当时语境，不能据此认定当前未发布；升级执行仓库最新迁移，不停在模块初始版本。

启用 fittings 模块后在此 57 项 SeAT 兼容基线上追加 `esi-fittings.write_fittings.v1`，合计 58 项。它是本项目新增的游戏配装写权限，不改动下方 SeAT 基线清单；开发者应用需增加此 scope，已有授权由本人重新授权。当前业务范围见[配装指南](fittings.zh-CN.md)。

更新：2026-09-14。[English](seat-login-scopes.en.md)。本文件替代前一阶段“仅申请本人军团职务 scope”的配置步骤。

## 获取范围

按 SeAT 的默认 `sso_scopes` 配置申请，而非申请 SSO 提供的所有能力。代码对应 [eveseat/web 的固定版本](https://github.com/eveseat/web/blob/cd11287006bddbf00c48cf13fd1fa704b0ea25d6/src/WebServiceProvider.php)，与 [SeAT 登录控制器](https://github.com/eveseat/web/blob/cd11287006bddbf00c48cf13fd1fa704b0ea25d6/src/Http/Controllers/Auth/SsoController.php) 一样，在混合私有权限时移除 `publicData`。

SeAT 原清单有 59 项；去除 `publicData`，再排除当前 [ESI OpenAPI](https://esi.evetech.net/meta/openapi.json?compatibility_date=2026-08-18) 未列出的 `esi-characters.read_chat_channels.v1`，本项目请求 **57 项**。两份清单已按固定提交逐项核对，其余项目保持一致。SSO 发现文档没有 `scopes_supported`，不能据此推断权限是否存在。

清单覆盖角色与军团资料、资产与钱包、合同与市场、技能与工业、邮件与通知等，也包括 SeAT 默认的 `esi-ui.open_window.v1`；不要把整套授权描述成严格只读。当前已实现多个同步模块与用户显式触发的配装写入；scope 不代表自动执行游戏操作。获得 scopes 不等于已经完成所有数据模块，也不改变本站 RBAC 的范围限制。

- [完整 57 项清单，可用于应用后台配置](seat-default-scopes.txt)
- 代码中的唯一请求清单：`internal/modules/eve/scopes.go`

使用一个内置默认授权方案；SeAT 的多方案管理、继承主角色 scope 和自动关联账号未引入。本站已实现[显式角色绑定](seat-multi-character.zh-CN.md)，绑定与重新授权使用当前本站方案。后续增加授权方案需要扩展服务器配置及流程记录，不接受浏览器任意指定 scopes。

## 配置与使用

1. 在 **现有 EVE 开发者应用**中允许上述 57 项基线 scopes；启用 fittings 时再增加 `esi-fittings.write_fittings.v1`。应用凭据和本机加密密钥保持原值，不需要重新生成。
2. 回调继续使用 `http://127.0.0.1:5173/api/v1/eve/callback`。
3. 执行 `npm run dev:external`，或单独部署时先 `npm run db:migrate` 再重启 API。
4. 访问 [独立登录页](http://127.0.0.1:5173/login)，在 CCP 页面确认范围。应用允许申请的 scope 与玩家实际同意的 scope 都必须满足；见 [官方 SSO 文档](https://developers.eveonline.com/docs/services/sso/)。

`/login` 不使用工作台侧栏或顶栏，正常状态仅显示品牌和 EVE 登录按钮，权限详情在 EVE 官方授权页确认。成功回调进入 `/account`；已登录访问 `/login` 自动进入角色页。取消或失败时留在登录页显示原因，即使此前已有会话也不会丢失错误提示。

`/account` 显示角色、军团职务和本站角色，提供退出与更新授权；未登录访问它会进入 `/login`。现有公开工作台/系统状态页没有在本轮改成全站登录门禁，业务数据 API 仍由服务端鉴权保护。

## 核验与升级

- 每次登录把实际请求的 scope 清单绑定到数据库中的一次性 flow，回调核对签名 JWT 包含原请求清单。中途配置变更不能降低原流程的校验标准。
- 验证成功后保存加密令牌及独立的已授予 scope 元数据；浏览器没有令牌访问权。`GET /api/v1/eve/status` 返回请求清单；`GET /api/v1/access/me` 的 `needs_authorization` 标识现有授权是否缺项。
- 原来的单一职务授权不会自动升级为 57 项，也不会凭空获得其他访问能力；角色页提示“更新 EVE 授权”。旧令牌可按原有范围刷新，职务同步仍遵循原有快照有效期。
- 刷新后范围不得少于该令牌原先实际获得的范围，不能因为全站方案扩大就使所有旧凭据立即失效，也不能静默接受权限丢失。
- `00006_eve_scope_profile.sql` 引入范围记录；多角色迁移为 `00007`，社区资料迁移 `00008` 使当前 Goose 为 8，foundation marker 仍为 1。旧记录的新增 scopes 元数据初始为空，下一次同步或重新授权更新。

本地集成测试覆盖完整/缺项 JWT、一次性 flow 范围快照和旧授权刷新；浏览器测试覆盖独立布局、角色页保护、成功/取消/退出跳转及移动端。真实 CCP 的应用 scopes 配置、玩家授权和回调仍待实际联调；本机能构造正确的授权地址不代表 CCP 已接受整套授权。
