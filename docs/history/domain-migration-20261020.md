# 正式域名迁移（2026-10-20）

用户将正式域名调整为 `seat.glorynavy.com`，旧 `seat.kisectool.com` 使用 CNAME 指向新域名，并要求重新配置 1Panel 网络。

## 定位

- 新旧域名均解析到现有边缘机。新域名由 1Panel 管理的证书已就绪，自动续期开启；有效期至 2027-01-18。
- 请求 `Q5RJKJ3SRUGEWWH5SK7RUSCU7X` 在应用日志中对应登录 POST 403。后端 `PUBLIC_ORIGIN` 仍是旧域名；同源校验严格比较 Origin，不根据代理 Host 推断，符合既有安全约定。
- 面板已将域名绑定及证书改到新域名，但站点主域名元数据仍旧；代理 Host、预警服务地址及旧证书续期钩子未同步。旧 HTTPS 域名缺少匹配服务器，TLS 握手失败。
- CNAME 不是网页跳转；浏览器继续携带访问地址的 Origin。无需放宽 CSRF、增加通配 CORS 或共享 Cookie。

## 配置范围

- 主站、代理 Host、应用 `PUBLIC_ORIGIN`、ESI 联系地址和 `SENTRY_INTEGRATION_URL` 统一为新域名；保留站点代号、发布目录、上游地址与全部凭据。
- 1Panel 主站仍使用其现有新域名证书和自动续期。旧域名单独配置 HTTP/HTTPS 301，保留普通路径与查询；旧写请求和旧 OAuth 回调以 303 回到新登录页，丢弃旧回调 code/state。
- 旧证书 Certbot 钩子只更新旧站 TLS 目录，避免覆盖 1Panel 新证书；旧站保留 ACME challenge。
- 不切换应用版本、迁移数据库或清空授权。新域名使用独立的 host-only Cookie，成员需要重新登录。
- 修改前备份环境、代理与 1Panel SQLite 快照；不开放面板 API。站点主域名仅更新已核对的单行元数据，不覆盖运行中的整个数据库。

## 验证与执行

配置来自干净且已推送的 main `e0aca9fa9e383b3c54fcb2f0a105ef5402e3b595`，于 2026-10-20 03:56 UTC 应用。GitNexus 暂存与相对 main 检查均 LOW，无受影响业务流程；本次只修改部署配置及说明，不修改鉴权函数。

- 应用环境备份：`/var/backups/glorynavy/domain-20261020T035644Z/seat.env`；边缘配置、旧证书和 1Panel 数据库快照：`/root/glorynavy-deploy/domain-20261020T035650Z/`。均在服务器受保护目录，没有导出到仓库。
- 1Panel 站点主域名已改为新域名，代号与证书关联保持一致。应用 `active/running`、`NRestarts=0`；后端保持 `v0.1.0-exchange-withdraw-20261019`，前端保持 `v0.1.0-workspace-logo-20261020`，公网 HTML 与既有发布产物字节一致。
- 新站 `/`、`/login`、`/workspace`、`/help` 为 200；HTTP 跳转 HTTPS。旧域名 HTTP/HTTPS 均通过证书验证并将普通路径及查询 301 到新站；旧 POST 和旧回调返回 303 新登录页，未携带旧 code/state。
- 未模拟生产 API 的实际 Edge 浏览器点击登录返回 303，Origin 与新站一致；SSO 回调为新域名、PKCE 为 S256，流程 Cookie 为 Secure/HttpOnly/SameSite=Lax 且仅限新主机。浏览器到达真实 EVE 登录表单，没有提交玩家凭据。
- `null`、旧域名、不可信域名 Origin 及 cross-site 登录请求仍为 403；模块、系统状态、EVE 状态、审批和钱包匿名请求仍为 401。
- 新域名下 Sentry 的两个服务端认证只读接口 `client-usage` 与 `monitor-contributions` 均为 200；未创建/轮换密钥、修改计费或发送消息。
- 实际执行旧证书部署钩子后，新证书 SHA-256 保持 `8b585bc3fa694b36427e2298a155118d4ffd08dbf258276204d16ecae814b436`；新旧 TLS 分别覆盖对应主机。旧站 HTTP/HTTPS ACME challenge 临时文件均 200，随后清理；OpenResty 配置校验通过。未进行证书重新签发。

以上自动检查只覆盖登录启动与配置，不等同于完整授权验收。随后用户曾收到 EVE 的回调地址不匹配错误；核对网站请求仍使用新回调后，用户在 EVE 应用保存该地址，并于 2026-10-20 明确回复“已经可以正常登录”。本轮完整登录验收因此已由用户实际操作确认，未由代理代填玩家凭据。检查脚本和无敏感值结果保存在忽略目录 `.local/domain-migration/`。

## 恢复边界

优先修正新域名配置。若确需恢复旧域名，必须一起恢复应用来源、对应代理与 HTTPS 站点以及 EVE 开发者应用回调，先核对当前证书覆盖范围；不能只把 `PUBLIC_ORIGIN` 改回去。恢复 1Panel 元数据时只按审计恢复对应字段，不覆盖整个运行中数据库。保留发布目录、所有授权、数据库和新旧证书，不用配置备份覆盖业务数据。
