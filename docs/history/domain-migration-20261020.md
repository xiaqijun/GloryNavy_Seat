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

仓库配置已准备，尚未应用。待核对 nginx 配置、新旧域名跳转、实际浏览器登录 Origin/303/PKCE/Cookie、匿名权限、旧/恶意 Origin 拒绝、上游健康和旧证书续期隔离。EVE 新回调预检查可到达官方登录表单；实际用户授权换码仍需真实登录确认。
