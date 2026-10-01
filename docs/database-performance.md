# 数据库查询性能与观测

## 应用慢查询日志（已随 v0.1.0-sync-retention-observation-20260923 发布）

DB_SLOW_QUERY_MS 默认 500，允许 0–60000 毫秒，0 关闭。配置进入主服务 pgx 连接池，覆盖通过该连接池的 Exec / Query / QueryRow 及事务查询。只记录达到阈值的完成事件，正常和失败查询都可观测。修改配置需重启应用，无需重启 PostgreSQL。LogLevel 若高于 warn 会过滤这类日志。

日志消息为 slow database query，字段为 query_name（合法 sqlc 注释名，否则 unnamed）、sql_hash（SQL 文本 SHA256 前 16 位）、duration_ms、rows、result，数据库错误另外记录 SQLSTATE。禁止输出 SQL 原文、绑定参数、连接串或数据库错误 Message/Detail/Hint。SQL 名称只供定位；指纹针对具体语句文本，不作跨版本/不同字面值的归一化标识。

该耗时包含客户端执行、网络和结果消费到 Rows 关闭的时间；不包含从连接池获取连接之前的等待，不是 PostgreSQL 纯执行时间。不覆盖单独迁移 CLI、SendBatch、CopyFrom 或其他服务的数据库连接，也不提供历史 SQL 的全量平均/P95。

在应用服务器查看（仅运维权限）：

```sh
journalctl -u glorynavy --since '1 hour ago' -o cat --no-pager | grep '"msg":"slow database query"'
```

## 数据库累计查询统计（运维方案，尚未启用）

若需要全量调用次数、平均/最大/累计数据库执行时间，可启用 PostgreSQL 18 的 pg_stat_statements：先检查并保留现有 shared_preload_libraries，加入 pg_stat_statements；安排数据库重启后，在业务数据库由 DBA 执行 CREATE EXTENSION pg_stat_statements。这一操作不包含在 Goose 46，不会随本轮应用升级自动重启数据库。

对外报告仅提供 queryid 和统计量；原始 query 文本只由受信运维读取，不能作为匿名 API 输出。数据库原生 duration 日志如需启用，应同时禁止参数打印，审查含字面量 SQL，避免记录授权材料。本轮先使用不记录原文的应用日志。

依据：[PostgreSQL 18 pg_stat_statements](https://www.postgresql.org/docs/18/pgstatstatements.html)、[并发建索引](https://www.postgresql.org/docs/18/sql-createindex.html)。

## 清理性能修复与验证

Goose 46 及清理语句详见[ESI 同步](integrations/esi-sync.zh-CN.md)。本地 PostgreSQL 16、隔离测试 schema，模拟 1,840,000 条未过期记录：旧候选 SELECT 394.107 ms，新清理 DELETE（0 行）0.475 ms；两个分支均使用保留期索引，无历史表 Seq Scan。该对照用于验证查询计划；生产为 PostgreSQL 18.6，生产复测见下文。

测试包括严格保留期边界、NULL 完成时间、未知结果类别、最老完成记录优先、全局批量上限、耗尽后重复执行、Goose Up/Down/部分迁移重试，以及日志阈值/参数和错误信息脱敏。大数据测试需显式设置 TEST_SYNC_RETENTION_PLAN=1，且 TEST_DATABASE_URL 必须指向隔离的本地测试环境；测试创建并删除独立 schema，不复用生产表。

原始排查记录：[生产慢查询检查](reviews/production-query-performance-2026-09-22.md)。合同列表、在线统计、物品估价的接口耗时仍需利用新观测继续分解，本轮不声称已解决这些接口全部瓶颈。

本轮验证已通过：sqlc 生成，保留期/批次/重复调度数据库测试，Goose 46 上下迁移与部分迁移重试，184 万行查询计划对照，慢查询阈值/配置/敏感信息保护测试及真实 pgx 查询钩子测试；Go vet 与主服务构建通过。以上为发布前本地验证。

## 生产发布验证（2026-09-23）

Goose 46 已完成，两个并发索引均有效，后端版本 v0.1.0-sync-retention-observation-20260923。生产新候选 SELECT 的 EXPLAIN ANALYZE 执行时间 1.059ms、规划 9.150ms，两个 Index Only Scan 合计 6 个缓冲块；此前旧候选 SELECT 为 571.715ms。两次均无过期候选，不是过期积压场景的性能保证。

确认有效阈值 500ms、日志级别 info；短时间观察无慢查询事件，不人为制造线上慢查询。后台控制调度及同步记录继续完成，无锁阻塞、无启动错误；数据库未重启，备份与回退位置见[项目状态](project-status.md)。
