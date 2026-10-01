# 预警扣费验收记录（2026-10-01）

## 已完成的技术验收

- EVE Sentry Windows 客户端声明 `alert-ack.v1`，SSE 请求带能力、连接 ID 和客户端 ID。
- 客户端只在告警通过本地去重并交给浮窗处理后，对携带完整 `delivery_id`、`charge_event_id`、`revision`、`connection_id`、UTC 起止时间和持续秒数的投递发送幂等 ACK。
- ACK 固定携带 `alert-use-evidence.v1` 快照（客户端、版本、主机、连接、接收/处理时间、去重结果和原始告警证据）；服务端缺少完整快照时不把预留转为已消费。
- Sentry 对账接口返回 ACK 证据，Seat 的 `AlertDelivery` 读取模型保留 `ack_at` 和 `ack_evidence`，与原投递、事件修订和区间引用一起核对。
- EVE Sentry 定向回归：`tests/test_seat_billing.py` 3 项通过；客户端 ACK、启动/重连和 HTTP 客户端回归 81 项通过、1 项跳过。
- Seat 侧已有隔离账本测试覆盖果壳币预留、区间结算、未确认释放、退款、退款后重预留、余额不足和状态分项读取；本轮通过仓库 `npm run check`，Go 全量测试（含 `internal/modules/exchange`、`internal/modules/sentry`）通过。
- 本轮预警端服务全量回归通过：`888 passed, 29 skipped`；客户端全量回归通过：`445 passed, 15 skipped`；客户端 ACK/启动/重连/HTTP 定向回归 `81 passed, 1 skipped`；Seat 前端 `91 passed`、构建通过，lint 仅保留既有福利面板两条 warning。
- 本地部署验收执行 `npm run db:migrate` 成功，Goose 已到 60，River 迁移也已完成；示例配置中的两个收费开关仍为关闭状态且没有生产费率默认值。

## 尚未通过的生产验收

正式价格按生产配置提供，不写死在代码中。启用前必须填写 `SENTRY_ALERT_PRICE_VERSION`、`SENTRY_ALERT_UNIT_SECONDS`、`SENTRY_ALERT_UNIT_PRICE_MINOR`、`SENTRY_ALERT_MAX_GRANT_SECONDS` 和 `SENTRY_ALERT_GRANT_TTL`；当前测试使用的 `price-v1`、每 60 秒 7 个最小币单位（0.07 果壳币）仍只是示例，不能自动写入生产环境。

真实生产币账现场验收未执行，当前也没有隔离生产账号：预留、ACK 结算、超时释放、退款、余额不足、并发重放以及 `/sentry` 与 `/exchange` 的余额/流水一致性仅完成代码和本地测试验证，不能标记为真实生产账验收通过。

因此可以推送收费关闭的代码和迁移，但 `SENTRY_ALERT_CONSUMPTION_ENABLED` 与 `EVE_SENTRY_SERVER_ALLOW_ALERT_CONSUMPTION` 继续保持关闭。若要在没有隔离生产账号实账验证的情况下开启收费，必须另行明确接受该风险；本记录不把“价格可配置”或本地测试等同于真实生产币账验收。

## 推送结果

EVE Sentry 收费能力代码已推送到远端 `main`，当前部署提交为 `7f20358ab58dba2a2e4bbe95fe77ad1808191582`。受保护生产工作流 `36894626384` 的质量、后端/前端验证、PostgreSQL 集成（98 项）、打包、部署和公网 `/api/readyz` 均通过；生产已运行收费代码。收费开关仍保持关闭，未发生真实扣费；正式价格配置和无隔离账号的真实币账验收仍未完成。
