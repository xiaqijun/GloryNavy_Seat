# EVE Sentry 预警平台密钥

状态：2026-10-03，本站第一阶段、单卡片更新、页面自动准备和卡片内显示/复制交互已发布生产。114 上的 Sentry 服务、`seat.kisectool.com` HTTPS 反代和 Seat 服务端凭据已通过无令牌 401 与带凭据参数校验；Sentry 端普通客户端鉴权的 M1 切片仍默认关闭，真实成员轮换仍待现场验收。`/sentry` 另有第一期本人只读的预警果壳币账单；收费规则与开关已加入 Goose 64，开关默认关闭并由管理员在前端配置。

成员打开 `/sentry` 时由页面自动准备一张默认密钥卡片。每个本站账号只保留一张当前密钥卡片；页面不展示用途分类，也不提供新建或删除入口。卡片使用正方形布局，密钥区域和复制按钮始终可见且可点击；首次准备或刷新成功后，完整明文直接显示在当前页面内存中，点击密钥区域或复制按钮即可复制。若当前会话没有明文，显示掩码并提示需要先生成或刷新，不会自动轮换或尝试恢复旧明文。刷新会直接轮换密钥，不再弹确认框。刷新成功后复用原卡片写入新前缀，更新后的完整密钥只显示一次；刷新失败时保留旧卡片状态。本站不会把明文写入数据库、浏览器存储、URL 或日志。历史已吊销记录仍保留在审计中，但不会在成员列表重复展示。

本站 API：

- `GET /api/v1/sentry/keys`：读取当前账号密钥。
- `POST /api/v1/sentry/keys`：创建密钥，JSON 为 `{name, permissions, request_key}`，其中 `permissions` 只能包含 `monitor`、`alert`，`request_key` 为 UUID 幂等键。
- `POST /api/v1/sentry/keys/{id}/rotate`：更新当前密钥。服务端保留本站卡片 ID，生成新的远端密钥并直接吊销旧远端密钥；成功响应包含一次性 `secret`，不接受名称或用途变更。
- `DELETE /api/v1/sentry/keys/{id}`：吊销本站记录对应的远端密钥。
- `GET /api/v1/sentry/alert-pricing`：读取历史预警授权计价投影；新收费不按事件或投递次数触发。
- `PUT /api/v1/sentry/alert-pricing`：保留给历史授权记录和审计，不用于新的在线时长收费配置。
- `GET/PUT /api/v1/sentry/time-pricing`：读取或保存按小时的预警消费价格、监控奖励价格和 `charging_enabled`；仅站点管理员可写入，开关与价格使用同一版本冲突保护，关闭时不创建新的收费授权。

新收费只读取预警端导出的 `GET /api/v1/integrations/seat/client-usage`：每条记录是
同一认证客户端相邻有效心跳之间的服务端确认在线区间，Seat 按区间秒数和当前小时价格
幂等结算。预警事件、投递和 ACK 记录可以继续被接收和查询，但不再作为新的收费来源。

保存 `charging_enabled` 时，Seat 服务端会在提交本地价格事务前调用预警端的
`PUT /api/v1/integrations/seat/alert-consumption`，请求为 `{"enabled":true|false}`，使用同一
服务令牌和幂等键。预警端把门禁写入 `seat_integration_settings`；远端调用失败时本地开关不会提交，
本地事务失败时会尝试恢复远端原状态。预警端的环境变量只用于新库首次启动的默认值，不能再作为页面开关。

预警授权使用 Sentry v2 秒数授权投影。Seat 冻结 `price_version`、`unit_seconds` 和 `unit_price_minor`，在本地预留、结算果壳币，并把这份冻结快照返回给 Seat 调用方；下游请求只包含操作/授权/账号标识、可选密钥 ID、`reserved_seconds`、有效期和 `protocol_version: 2`，不发送价格字段。v1 价格字段仅为历史兼容保留。

创建接口在账号已有未吊销密钥时返回冲突；页面只在没有当前卡片时自动调用创建接口，需要换密钥时点击卡片内的“刷新密钥”。

创建会先把本地状态记为 `creating`，再直接调用远端；远端确认返回匹配的 `key_id`、协议版本 1 和 `active` 状态后才变成 `active`。网络失败会留下 `sync_error`，不会显示成功，也不会生成另一把密钥；使用原 `request_key` 重试会再次直接调用远端幂等创建接口。相同内容重试返回原操作（HTTP 200，不再次返回明文），内容变化会冲突。刷新仍由服务端先创建新远端密钥、再吊销旧远端密钥；若结果未确认，卡片保留同步错误供后续处理。吊销 API 保留给受保护的服务端兼容与审计流程，成员页面不提供吊销按钮，历史审计不会删除。

远端配置只放在服务端环境文件：

```dotenv
SENTRY_INTEGRATION_URL=https://sentry.example.com
SENTRY_INTEGRATION_TOKEN=<server-only-token-at-least-32-chars>
```

EVE Sentry 已提供 `POST /api/v1/integrations/seat/keys`、`DELETE /api/v1/integrations/seat/keys/{key_id}`、
`PUT /api/v1/integrations/seat/alert-consumption`，以及预警事件、投递和监控贡献的读写/对账接口。
调用使用独立 Bearer 凭据和 `Idempotency-Key`；本站发送 `operation_id`、密钥 ID、账号 ID、密钥哈希、前缀、用途和协议版本，不发送明文。
Sentry 只保存哈希并按 `operation_id` 幂等处理：相同内容返回原记录，内容变化返回冲突；未配置或不可用时本站返回“预警平台密钥服务暂未配置或不可用”。
生产 HTTPS 入口仍以 `https://seat.kisectool.com` 为 origin，必须将整个 `/api/v1/integrations/seat/` 前缀反代到 114；服务令牌只保存在两端受限环境文件中，不进入文档、浏览器或 River 载荷。

Sentry M1 增加了独立的 `auth_external_accounts` 显式绑定：Seat `account_id` 不等同于 Sentry 的本地用户 ID，必须由受信管理流程一对一绑定。`EVE_SENTRY_SERVER_SEAT_AUTH_MODE` 默认 `off`，`shadow` 只记录校验并拒绝，`enforce` 才按 `monitor`/`alert` 白名单建立业务 principal；未绑定、已吊销、已禁用或越权请求会稳定拒绝。该能力尚未由本站开启，也未完成生产联调，不得把密钥申请页面当作预警客户端已可用的证明。

本阶段不读取截图、在线时长、质量分或预警事件；实际预警扣费由 Seat 页面开关和预警端持久化门禁共同控制，发布默认关闭，当前生产状态以两端同步结果为准。
`SENTRY_ALERT_CONSUMPTION_ENABLED` 仅保留为兼容配置校验，不再是页面开关。监控奖励和预警计费继续按
[EVE Sentry 接入方案](../plans/eve-sentry-integration.zh-CN.md) 分期实施。密钥申请或收费规则配置可用不等同于预警客户端鉴权或消费闭环已完成。
