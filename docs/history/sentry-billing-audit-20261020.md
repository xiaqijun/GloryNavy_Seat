# 预警收费只读核查（2026-10-20）

## 结论与范围

2026-10-20 12:51（北京时间）起核对 Seat 与 EVE Sentry 生产数据库、集成 GET 接口和 Sentry 服务日志。查询使用只读事务；未创建心跳或收费测试数据，未修改开关、价格、数据库结构、余额或历史流水，未发布程序。

收费存在故障，不能视为已完成真实计费验收：Sentry 收到认证预警客户端心跳，但消费证据写入失败；Seat 读取成功且已经追平现存证据。另发现短区间逐笔向上取整导致累计费用高于按合计时长计算的金额。

## 配置与对账

- Seat `sentry_alert_pricing`：收费开启，`hourly-v13`，`unit_seconds=3600`、`unit_price_minor=200`，即每星系每小时 2 果壳币；监控奖励同为每小时 2 果壳币。价格于 2026-10-10 生效。
- Sentry `GET /api/v1/integrations/seat/alert-consumption` 返回 `enabled=true`，两端开关一致；新域名的 `client-usage`、`monitor-contributions` GET 接口均成功。
- Sentry 共保存 1,841 条消费区间、24,955 秒；Seat 合并为 465 条已结算区间，秒数相等，总扣费 1,609 最小单位，即 16.09 果壳币。历史单价随冻结版本不同，不能用当前 2 果壳币/小时重算旧账。
- 465 条预留币账与 grant 的账号、请求键、引用和金额逐笔一致；charge 与 grant 的已结算秒数/金额一致。未发现重复 `interval_id`、区间长度错误、相对冻结单价的公式错误或未结清预留余额。
- 最后消费区间结束于 **2026-10-09 01:53:59**，最后扣费入账于 **01:54:19**；近七日无扣费。Seat 游标与 Sentry committed watermark 一致，后续 GET 返回空页、`has_more=false`，不是 Seat 未消费现有积压。
- 监控奖励最近入账于 **2026-10-20 11:06:14**，对应区间结束于 11:05:18。奖励同步与消费证据是独立路径，不能用奖励正常证明扣费正常。
- River 当前仍按分钟尝试同步。任务保留域名切换期间的历史远端错误，但本次 GET 成功；这些错误不能解释 10 月 9 日起的消费证据缺失。

## 消费证据写入失败

Sentry 当前程序 `app/server/seat_billing.py` 的 `record_client_usage` 使用：

```sql
ON CONFLICT (account_id, key_id, client_id, system_id, started_at, ended_at)
DO NOTHING
```

生产 `seat_client_usage` 的同名索引却仍是：

```sql
CREATE UNIQUE INDEX idx_seat_client_usage_identity
ON public.seat_client_usage (client_id, system_id, started_at, ended_at)
```

不存在匹配新冲突目标的唯一约束/索引。2026-10-20 11:05 左右的真实服务日志反复记录：

```text
psycopg.errors.InvalidColumnReference:
there is no unique or exclusion constraint matching the ON CONFLICT specification
```

调用链为 `PostgresIntelStore.record_heartbeat` → `SeatBillingRepository.record_client_usage` → INSERT。同日数据库保存了具有 Seat 账号/密钥归属的 `alert_client` 心跳，排除了“只有监控节点、没有预警客户端”这一解释。

当前迁移使用 `CREATE UNIQUE INDEX IF NOT EXISTS idx_seat_client_usage_identity ...`；旧同名索引已经存在时不会替换其列定义。后续修复需校验实际索引定义并补齐兼容迁移，而非仅再次执行同一条 `IF NOT EXISTS`。恢复前要验证旧数据重复、隔离账号/密钥、迁移重复运行与真实 PostgreSQL 行为。

## 取整与星系归因验收缺口

Seat 每个结算区间按 `ceil(seconds × unit_price_minor / unit_seconds)` 独立取整，跨页没有保留小数余量。以 `hourly-v10` 实账为例：356 条、21,949 秒，冻结单价 1 果壳币/小时；实际扣 7.34 果壳币，合计时长仅取整一次为 6.10，差 1.24。逐笔算术符合当前代码，但累计收费随分段方式变化，需修正或明确计价口径；本轮未调整旧账。

全部 1,841 条既有远端消费证据的 `system_id` 为空。不能把 `COUNT(DISTINCT system_id)=1` 解读为已验证一个真实星系；多星系分别收费与选中范围仍须真实验证，不能只依据发布说明宣称通过。

待办见 [SENTRY-01](../backlog.md)。缺失区间没有可靠消费证据，本轮未按监控奖励、最后心跳或经过的自然时间推算补扣。
