# 存储保留与清理

2026-10-17 随 `v0.1.0-storage-retention-20261017` 发布；首次执行结果以[项目状态](project-status.md)及[部署记录](deployment.md)为准。

## 文件轮换

`deploy/rotate-storage.py` 默认仅生成 JSON 清单，显式 `--apply` 才删除。只处理项目的发布目录和根目录下完整的 `.dump` / `.env` 备份对及其 `.meta.json`，不处理业务表、Docker 卷、日志、临时文件或历史备份子目录。

- 备份保留最近 3 份；最近 7 天按 UTC 每日留最新一份，7–28 天按 ISO 周留最新一份，超过 28 天按月留最新一份。密钥配置与数据库备份成对保留，缺配置、空文件和未知布局保持原样。
- 发布包保留当前及按 `release.json.built_at` 排序的最近 5 版。应用机额外保留所有留存备份对应的 `backend_release`；旧备份没有元数据时，保留名称中的目标版本和备份前最近两份构建。应用发布包自带同版本前端，供历史恢复使用。保留不等于允许跨数据库版本直接回退。
- 缺少版本元数据/校验清单、版本名不符、包含符号链接的目录不自动删除；根目录及 current 必须通过真实路径范围校验。
- `/etc/glorynavy/storage-retention-pins.json` 可额外保留指定名称，例如 `{"releases":["v0.1.0-example"],"backups":["before-example.dump"]}`。该文件不包含密钥。
- 实际删除前，所有保留的完整备份通过 `pg_restore --file=/dev/null` 完整读取，保留发布包核验 SHA256SUMS；核验失败则不开始删除。核验归档可读性不能替代定期隔离恢复演练。
- 计划前后重新核对清单与文件身份；与应用/边缘激活脚本共享 `/run/lock/glorynavy-storage.lock`。清单、源码提交号和完成标志保存在 `/var/log/glorynavy-storage-retention/`，0600，不记录配置内容。

在对应 Linux 主机执行：

```sh
python3 deploy/rotate-storage.py --role app
python3 deploy/rotate-storage.py --role edge
# 检查清单后，在对应主机显式应用
python3 deploy/rotate-storage.py --role app --apply
python3 deploy/rotate-storage.py --role edge --apply
```

应用机固定范围为 `/opt/glorynavy/releases` 和 `/var/backups/glorynavy`；边缘固定范围为 `/opt/1panel/www/sites/seat.kisectool.com/releases`。根目录外的符号链接目标不会被删除。

首次核验清理后，分别用 `bash deploy/install-storage-retention.sh app` / `edge` 安装维护脚本和 systemd 定时器。每周日 20:20 UTC（周一北京时间 04:20）执行，最多随机延迟 15 分钟；离线错过后补执行。查看 `systemctl list-timers 'glorynavy-storage-retention@*'` 与相应 service 的日志。停止自动清理用 `systemctl disable --now glorynavy-storage-retention@app.timer`（边缘替换为 edge）；手动固定清单不被停用命令清除。更新轮换工具后需重新运行安装脚本。

## 同步日志

仅 `eve_sync_runs` 的已结束执行记录使用新保留期：success 7 天，deferred/progress 3 天，其他 outcome 30 天。按 `finished_at` 而非开始时间计算，边界当天保留，未结束记录始终保留。后台每 30 秒仍最多删除 500 条，按完成时间、ID 全局取最早批次；沿用 Goose 46 两个部分索引，无新迁移。

积压可用 `sync-history-cleanup --apply --max-batches 30000 --pause 20ms` 补清。命令从环境读取 `DATABASE_URL`，使用 EVE 模块私有查询，单连接，单语句 10 秒、等待锁 2 秒；总运行最多 2 小时，每批最多 500 条且间隔至少 10ms。只输出批次/删除数量，失败、超限或中断后可重跑。它不会删除同步目标、租约、当前快照、缓存、在线样本和业务审计。

不要把数据库连接串写在命令行或提交到文档。生产使用已有受保护的 EnvironmentFile 启动该维护命令。后台调度也会取最早批次，并发时一次删除 0 行可能只是候选已被另一事务删除；补清返回后必须复查过期记录并按需续跑，不能只凭退出码判断积压清空。开始时记录数据库时间，以该固定时间减去各保留期复查原积压；运行中持续新到期的少量记录交回每 30 秒的后台维护，不要求动态计数永久为 0。

删行后使用普通 `VACUUM (ANALYZE) eve_sync_runs` 回收内部可复用空间，不承诺立即缩小文件；`VACUUM FULL` 会锁表，本流程不执行。钱包、贷款、合同交付占用、PAP、币账及业务审计不在清理范围。既有在线样本 31 天、缓存过期后 1 天及 River 清理保持原规则。

## 验证

Linux 文件轮换测试：`python3 -m unittest discover -s deploy -p test_rotate_storage.py -v`。覆盖保留桶、当前版本/固定版本/备份恢复版本、缺材料和符号链接保护、过期计划拒绝、校验失败不删除、精确候选及幂等。

数据库使用独立 PostgreSQL，通过 `TEST_DATABASE_URL` 执行 `TEST_SYNC_RETENTION_PLAN=1 go test ./internal/modules/eve -run 'SyncRunRetention|CleanSyncHistory' -count=1`；184 万行查询计划测试只能在隔离测试库运行。生产清理前后记录磁盘、目录与记录数，确认服务就绪，并保留至少一份实际恢复验证过的备份。

## 审批通知回执（Goose 92）

community 的分钟扫描将五分钟前遗留 sending 标为 unknown，清理 90 天前的非活动提醒回执和发送批次；持续待办去重回执保留，避免重发。业务审批、币账和原审计不在此范围。停用 community/approval 模块会暂停扫描和这项保留策略。
