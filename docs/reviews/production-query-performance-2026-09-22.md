# 生产查询性能检查（2026-09-22）

检查时间：北京时间约 23:35–23:42。只读检查生产 PostgreSQL 18.6、pg_stat_activity、统计表、过去 24 小时服务日志；对 SELECT 执行 EXPLAIN (ANALYZE, BUFFERS, TIMING OFF)，会话设为只读并限制 5 秒。未执行 DELETE/UPDATE 的 ANALYZE，未修改生产配置、索引、任务或重启服务。

## 已确认问题

同步运行记录清理 CleanupSyncRuns 的候选 SELECT 实测 **571.715 ms**。约 184 万行，Parallel Seq Scan 共三个执行实例，每个过滤约 614,894 行，结果为 0 行。Buffers shared hit 14,357 / read 9,289；read 是 PostgreSQL 缓冲区读入，不能等同于物理磁盘读取。eve_sync_runs 约 392 MB，全库约 677 MB。

生产索引仅覆盖主键 id、(target_id,fence)、(target_id,id DESC)，没有清理条件 finished_at 的索引。源码 internal/modules/eve/internal/store/queries/sync.sql 的 CleanupSyncRuns 按成功 14 天、其他 30 天保留期过滤，再按 id 取 500 条。internal/modules/eve/sync.go 中 30 秒调度调用 dispatch → qCleanup → CleanupSyncRuns；即使没有过期记录，仍扫描大表。生产实时采样也捕获到该查询，采样瞬间已执行 165 ms；这是活跃时长下界，不是完整查询耗时。

同表普通按目标读取最新 20 条记录使用 eve_sync_run_history 索引，受控查询耗时 **0.632 ms**，不是所有同步查询都慢。

## 其他实测

- 25 轮约 1 秒间隔活跃采样没有阻塞样本；即时检查没有长事务/锁等待。pg_stat_database 累计 deadlocks=0，无法证明从未出现短暂等待。
- 数据库缓存累计命中率约 93.64%；统计无明确 reset 时间，不能当成当天指标。累计临时文件 183 个、约 274 MiB，不能直接推断当前内存不足。
- 个人钱包本月流水选记录较多的角色做只读查询，走 eve_wallet_history 索引并 top-N 排序，处理 1,123 条后返回 51 条。总执行 58.947 ms，包含诊断时选取该角色的额外子查询，不是原 API SQL 的精确耗时；无证据称钱包存在秒级 SQL。

## 过去 24 小时接口日志

共 3,106 条 HTTP 请求日志，12 条 >=500 ms。以下为应用层总耗时，包括鉴权、SQL、处理及可能的外部请求，不是数据库慢 SQL 排名。

| 接口 | 次数 | 平均 ms | P95 ms | 最大 ms |
| --- | ---: | ---: | ---: | ---: |
| 物品估价 /api/v1/market/estimate | 4 | 7147.2 | 13331 | 13331 |
| 合同列表 /api/v1/eve/contracts/{kind}/{id} | 7 | 1092.7 | 1860 | 1860 |
| 在线统计 /api/v1/attendance/online | 37 | 335.6 | 1595 | 1832 |
| 钱包记录 /api/v1/wallet/records | 678 | 59.1 | 188 | 431 |
| 同步目标 /api/v1/eve/sync/targets | 4 | 145.2 | 417 | 417 |
| 审批队列 /api/v1/approval/items | 28 | 196.5 | 314 | 357 |

样本少的接口 P95 不适合推断长期表现。估价、合同列表和在线统计需要后续分段计时才能确认瓶颈。

## 观测缺口与建议顺序

生产 log_min_duration_statement=-1、log_min_duration_sample=-1、shared_preload_libraries 为空，pg_extension 只有 plpgsql；没有 pg_stat_statements，track_io_timing/log_lock_waits 均关闭。现有数据库日志没有 duration 记录。无法回溯全部 SQL 的平均、P95、最慢耗时或累计成本。

1. 优先优化同步历史清理：评估 finished_at 索引或按结果类别拆分的索引与候选查询；降低维护执行频率。保持原有保留期和每批上限，先验证计划，再通过迁移交付。
2. 增加安全的慢查询及累计耗时观测，避免记录令牌/敏感绑定参数；pg_stat_statements 的 preload 配置需要单独安排数据库重启，不在本次只读检查中执行。
3. 分析合同列表和在线统计的 SQL、应用处理与调用次数；物品估价区分外部价格读取耗时。
4. 钱包可继续评估工作台按角色分页拉取整月流水带来的请求量，考虑服务端聚合；这属于请求放大风险，不能凭当前证据认定为慢 SQL。

本轮只交付诊断结果，未实施性能修复。
