# 历史记录与文档清理

更新：2026-09-23。当前能力看[项目状态](../project-status.md)，下一步看[待办](../backlog.md)，本目录不作为现行部署指令。

- [截至 2026-09-23 的交付历史](project-status-through-2026-09-23.md)：从原项目状态文档归档，保留发布版本、备份位置、验证结果与当时的限制；历史“未发布”不代表今天未上线。
- [工程底座](phase-1.md)、[登录](phase-2-login.md)、[权限](phase-3-authorization.md)：保留初始验收和决策依据。
- [SeAT 调研](seat-research-2026-09-13.md)、[数据库调研](database-access-research-2026-09-13.md)、[PAP 草案](pap-design-draft.md)、[配装排版调研](fittings-research-2026-09-15.md)：保留早期来源和方案依据。
- `docs/plans/`、带日期的调研/审查与对应截图：保留来源、已批准方案和验证证据；变更后的规则以模块指南与页面说明为准。

## 2026-09-23 清理

删除已被正式迁移和模块实现取代的以下 8 个草稿文件，不删除任何正式迁移、业务数据、截图或第三方来源证明：

- `docs/drafts/auth/README.md`、`00002_auth.sql.draft`、`queries.sql.draft`：已由 identity / eve / community 正式设计取代。
- `docs/drafts/attendance/README.md`、`README.en.md`、`implementation/00016_attendance.sql`、`implementation/attendance.sql`、`implementation/online.sql`：已由正式考勤指南、Goose 21 起的迁移和模块私有查询取代。

另外删除 `docs/drafts/exchange/README.md`：只有早期需求转述，且“手动转换未采用”与当前手动/自动模式相矛盾。兑换事实集中到[正式指南](../integrations/exchange.zh-CN.md)。合计删除 9 个文件。

保留 [PAP 参考草案](pap-design-draft.md)，因为包含固定 SeAT 插件提交及原始调研来源；该文件明确标记为历史设计，不参与迁移。删除文件的引用已改指正式指南或此清理说明。

本轮仅整理文档与废弃的非执行 SQL 草稿；没有应用或回退迁移，没有更改生产服务器。

## 本轮验证

2026-09-23：检查仓库根目录及 docs 下 107 份 Markdown 的 1152 个本地链接和锚点，未发现失效目标；核对 23 对中英文模块指南、根目录/前端脚本命令、发布脚本路径、当前迁移上限 46 和技术栈依赖。CHANGELOG 仅保留一个当前“未发布”入口，早期清单明确标为历史。

本轮没有重新联网核验外部参考资料、运行整套业务测试、操作游戏合同或部署生产；历史验证结果不计作本轮验收。文档扫描与对照记录保存在本机 `.local/doc-refresh/`，不包含服务器配置或凭据。

## 2026-10-03 文档收敛

阶段验收、SeAT/数据库调研、PAP 草案和配装排版调研移入本目录。主目录只保留当前状态、待办、开发/部署、架构、技术栈和通用规则；双语集成契约、页面说明、方案和验收证据继续保留在原目录。
