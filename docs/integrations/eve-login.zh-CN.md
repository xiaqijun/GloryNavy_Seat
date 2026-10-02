# EVE 登录配置与实现

> 状态索引（2026-09-23）：本指南对应的业务已上线；未交付的扩展与真实验收缺口见[待办](../backlog.md)，当前版本见[项目状态](../project-status.md)。下文带日期/Goose 的发布前记录保留当时语境，不能据此认定当前未发布；升级执行仓库最新迁移，不停在模块初始版本。

2026-09-21：受保护路由的 3 秒预算只用于会话读取、资料门禁、权限检查和续期。鉴权完成后，业务接收原请求的截止时间与取消信号及已验证身份，不继承鉴权子上下文；业务自行限制网络核价等操作时间。无会话迁移或强制重新登录。

更新：2026-09-14。环境：国际服 Tranquility。对应 [English guide](eve-login.en.md)。

已实现独立 EVE 登录页、本站会话、职务同步与 SeAT 风格权限。启用 access 时按 SeAT 默认清单的当前 ESI 兼容集合请求 57 项 scopes，加密保存令牌；其他数据模块并未因此自动实现。最新配置以 [默认授权指南](seat-login-scopes.zh-CN.md) 为准。

## 1. 创建开发者应用

从 [EVE 官方 SSO 文档](https://developers.eveonline.com/docs/services/sso/) 的 **My Applications** 入口进入应用管理，创建后端可保管 Client Secret 的应用，取得 Client ID 和 Client Secret。

本地回调地址登记为：

```text
http://127.0.0.1:5173/api/v1/eve/callback
```

与应用配置严格一致，不把 `localhost` 和 `127.0.0.1` 混用，不使用 API 内部端口 8080 作为前端入口。生产为 `https://你的域名/api/v1/eve/callback`，需要单独配置对应环境的应用。

当前生产部署目标回调为 `https://seat.kisectool.com/api/v1/eve/callback`，`PUBLIC_ORIGIN=https://seat.kisectool.com`。凭据仅在应用服务器配置；上线复用已有开发者应用时必须先修改其回调，原本地回调可能不再可用。开发和生产长期并行应使用各自的应用。域名、证书及具体部署验收见[生产部署](../deployment.md)与[项目状态](../project-status.md)。

当前已配置独立生产应用，开发环境继续保留原应用。生产凭据保存在应用服务器 `/etc/glorynavy/seat.env`，替换后重启 `glorynavy.service`；不得顺带重置 `EVE_TOKEN_KEY`。配置存在和跳转参数正确不代表 Client Secret 已通过真实换码验证。

生产反代的 HTML 页面使用 `Referrer-Policy: same-origin`，API/回调继续 `no-referrer`。若原生登录表单被页面级 `no-referrer` 影响而发送 `Origin: null`，后端会返回 `csrf_failed`；应修正页面来源策略并刷新页面，不放宽后端来源校验。上线验证须实际点击登录按钮，见[部署检查](../deployment.md)。

## 2. 配置本机

修改根目录 `.env`：

```dotenv
MODULES=system,identity,eve,access,community
PUBLIC_ORIGIN=http://127.0.0.1:5173
EVE_CLIENT_ID=填写应用的Client_ID
EVE_CLIENT_SECRET=填写应用的Client_Secret
EVE_TOKEN_KEY=
```

`PUBLIC_ORIGIN` 只包含协议、主机和端口，不带末尾斜杠或路径。回调地址由它加上 `/api/v1/eve/callback` 得出。生产必须 HTTPS，只有 loopback 开发允许 HTTP。

不要把凭据写入 `VITE_*`、前端代码或提交到仓库。两个凭据都为空时系统仍能启动，但登录页提示尚未配置；只填一个时启动配置校验失败。`configured=true` 仅表示凭据存在，不代表已经通过 CCP 验证。

在现有 EVE 应用允许 [完整 57 项权限](seat-default-scopes.txt)。仅在缺少密钥时运行 `npm run auth:key`，已有凭据及密钥保持不变，再重启开发服务：

```sh
npm run dev:external
```

有 Docker 的标准开发环境使用 `npm run dev`。这两个命令先显式运行迁移，再构建并启动。API 本身不会自行迁移；分开部署时先执行 `npm run db:migrate`。

浏览器打开 [独立登录页](http://127.0.0.1:5173/login)，也可从工作台顶部图标进入。成功登录跳转 `/account`；未登录访问角色页会返回 `/login`。

## 3. 登录流程与保护

1. 浏览器从本站表单 POST `/api/v1/eve/login`。后端核对 Origin；创建 10 分钟一次性 state、浏览器关联 Cookie、PKCE S256 verifier，并绑定当时的本站会话。
2. 后端从 [官方发现端点](https://login.eveonline.com/.well-known/oauth-authorization-server) 获取授权、token 与 JWKS 地址并缓存 1 小时，仅接受指定 EVE HTTPS 主机及 issuer。
3. EVE 回调先原子消费匹配的有效 state，再使用保密客户端 HTTP Basic 和 PKCE verifier 换取 token。缺失 Cookie、会话改变、过期或重复回调都不能登录。
4. 使用 JWT 库检查 RS256 签名、可信 JWKS、精确 issuer、过期/生效/签发时间，以及同时包含本应用和 `EVE Online` 的 audience；检查角色 ID、名字、owner 和出现时的 azp/tenant。
5. 验证通过后创建或识别本站账号，生成新的随机会话。数据库仅保存会话令牌哈希；Cookie 为 HttpOnly、SameSite=Lax，HTTPS 下为 Secure，不指定 Domain。
6. 角色 owner 发生变化时阻止登录、隔离原绑定并撤销该角色的旧会话，不继承原用户权限。当前没有管理员解封界面，需后续受审计的人工核对流程。

JWT、授权码、Cookie、数据库错误和 client secret 不记录到请求日志。回调立即跳转到固定本站页面，错误文案由固定错误码映射；接口返回 `Cache-Control: no-store` 和 `Referrer-Policy: no-referrer`。

## 4. 本站会话与模块

- `identity` 持有账号、角色与会话；`eve` 持有登录事务、加密令牌与职务同步；`access` 持有本站角色和授权策略。模块通过宿主注入的服务协作。
- 会话采用 7 天滑动有效期（2026-09-17 本地调整）：本站会话读取和通过会话、入口权限及 CSRF 校验的业务请求自动续期，同时更新 Cookie；频繁请求合并为最多每 5 分钟写一次续期，接口返回实际 `expires_at`。旧的 12 小时会话尚未失效时也可通过访问自动续期，不必重新登录；已过期、已退出或已撤销的会话不恢复。续期只更新到期时间，不轮换 token/CSRF，不延长 SSO state 或合并预览，也不改变 EVE 游戏令牌期限。
- 同一账号最多保留最近 5 个会话；重新登录仍替换当前浏览器旧会话。绑定与重新授权不直接重置登录会话，其普通本站访问适用上述续期。跨站探测、入口权限/CSRF 拒绝和退出请求不续期；无浏览器访问的 ESI 后台任务不延长会话。会话端点也将 Cookie 剩余时间对齐数据库，恢复丢失续期响应后的 Cookie。已实现显式多角色绑定，账号合并见[合并指南](account-merge.zh-CN.md)，多角色行为见[多角色说明](seat-multi-character.zh-CN.md)。
- GET `/api/v1/identity/session` 返回匿名状态，或认证角色 `character`、展示主角色 `main_character`、到期时间与 CSRF token；不返回会话凭据或 EVE token。
- POST `/api/v1/identity/logout` 要求本站 Origin、有效会话及 `X-CSRF-Token`。成功后撤销数据库会话、清 Cookie，并清除前端身份相关缓存。
- 宿主统一验证会话与 CSRF，access 实施本站角色、军团职务及对象过滤；未知能力拒绝。现有匿名运维接口不包含成员业务数据。权限管理页面已实现，Squads 尚未实现。
- 登录状态不等于 QQ/KOOK 确认。community 已实现手填资料和受保护业务的完整度门禁，确认记录与资料分离；机器人和外部确认入口待实现，详见[社区资料](community-profile.zh-CN.md)。

登录迁移为 00002/00003，职务授权为 00004/00005，范围记录新增 00006，多角色新增 00007，社区资料新增 00008；这些是登录相关的初始迁移，当前 Goose 版本见项目状态，foundation marker 仍为 1；旧草稿已清理。

开始登录时清理至多 1000 条过期流程；创建会话时清理至多 1000 条过期会话，过期记录无论是否物理清除都不能使用。长期维护可增加定时清理。当前限流按直接连接对端 IP，每 10 分钟 20 次启动尝试；不信任客户端伪造的代理头。生产反向代理上线前需配置经过信任边界校验的真实 IP 或调整限流策略，否则同一代理出口共享额度。

## 5. 验证与后续

登录与私有 ESI 已用于真实业务；早期模拟提供方、ESI 和 RSA 测试仅证明对应测试场景。新增 scope 仍需角色本人重新授权，真实配装写入、合同交付和最新生产登录表单复核等验收缺口见[待办](../backlog.md)。

配置后检查：登录取得正确角色；刷新页面保留会话；退出后会话不可用；取消授权可重试；两个标签页互换回调不能串账号。不要把已登录直接视为军团成员或授予管理员权限。

职务授权、令牌加密/刷新/撤销的当前实现见 [军团权限指南](seat-authorization.zh-CN.md)，其他 ESI 数据和 SDE 设计见 [完整接入指南](eve-integration.zh-CN.md)。本次没有调用游戏写入接口。
