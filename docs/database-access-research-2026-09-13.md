# Go 数据访问方案调研

日期：2026-09-13。范围：GloryNavy 军团平台，Go、PostgreSQL、River、4C4G 单机部署。

本文基于官方文档进行功能与架构比较，没有执行业务压测，不提供未经测量的吞吐量或内存排名。结论是选型建议，不代表用户已确认替换组件。

## 结论

建议保留 pgx v5 + sqlc + Goose。理由是本项目明确采用 PostgreSQL，需要控制数据同步、统计查询与任务事务，直接维护 SQL 与现有架构衔接较简单。

4C4G 不是排除 ORM 的理由。若优先快速实现 CRUD、动态筛选和关联加载，GORM 是可行备选；若强调类型化实体关系，可考虑 Ent；若偏好 SQL 风格的链式查询，Bun 是折中选择。以下效率评价是基于 API 形式对本项目的判断，不是所有团队的通用排名。

## 方案比较

| 方案 | 使用方式 | 适合的工作 | 本项目主要代价 |
| --- | --- | --- | --- |
| pgx + sqlc | 手写 SQL，生成 Go 参数、结果类型及调用方法 | 固定查询、统计、明确的更新与事务边界 | 每种查询需要 SQL；大量自由组合筛选需要额外设计 |
| GORM | Go 模型、链式条件、关联和生命周期钩子 | 常规后台 CRUD、动态筛选、快速迭代 | 需要理解零值更新、关联、钩子和事务的实际行为 |
| Ent | Go 定义实体及关系，生成查询 API | 实体关系复杂、希望字段与关系调用有编译检查 | 引入实体定义和生成流程；定制 SQL 需要学习扩展接口 |
| Bun | SQL 风格查询构造器、结构体映射、关系加载 | 动态查询、熟悉 SQL 的团队 | 查询片段仍常用字符串，不能获得 sqlc 对固定 SQL 的同类分析 |

功能依据：[sqlc 代码生成](https://github.com/sqlc-dev/sqlc/blob/main/docs/howto/generate.md)、[GORM 功能](https://gorm.io/docs/)、[Ent 类型化生成 API](https://entgo.io/)、[Bun 工作方式](https://bun.uptrace.dev/guide/)。

## 类型安全和开发效率

sqlc 在生成阶段解析 SQL 与表结构，产出普通 Go 方法；它不保证业务逻辑、对象权限或查询性能正确。修改结构后重新生成有助于暴露部分不匹配，但仍需数据库集成验证。[sqlc 生成文档](https://github.com/sqlc-dev/sqlc/blob/main/docs/howto/generate.md)

GORM 已提供泛型 API，不能再简单归类为“完全没有类型安全”。不过诸如 Where("name = ?", value) 的字段字符串不会仅因使用泛型而被 Go 编译器验证。GORM 官方建议新代码使用泛型 API。[GORM 泛型](https://gorm.io/docs/the_generics_way.html)

Ent 会生成实体字段、关系和查询方法，对关系操作的类型化支持是其优势；它也支持聚合及自定义 SQL 选择器，不能说 ORM 无法做复杂统计。[Ent](https://entgo.io/)、[Ent 聚合](https://entgo.io/docs/aggregate/)

本项目的具体取舍：成员审核、公告等 CRUD 使用 GORM 可少写一些 SQL；舰队出勤汇总、补损统计、资产筛选采用 sqlc 更容易直接阅读和调整最终 SQL。若以后出现大量可自由组合的筛选条件，可以局部增加参数化查询构造，而不必一开始维护两套主数据访问体系。排序字段应使用白名单，不能直接拼接用户输入。

## 与 River 的连接和事务

River 推荐 PostgreSQL 使用 riverpgxv5。sqlc 的 pgx 生成代码可以通过 WithTx 使用 pgx.Tx，因此业务写入与 InsertTx 入队可置于同一事务。比如角色绑定记录与首次同步任务一起提交或回滚。[River 驱动](https://riverqueue.com/docs/database-drivers)、[sqlc 事务](https://github.com/sqlc-dev/sqlc/blob/main/docs/howto/transactions.md)、[River 事务入队](https://riverqueue.com/docs/transactional-enqueueing)

GORM 和 Bun 也能通过 riverdatabasesql 共享 *sql.DB 和 *sql.Tx，有官方接入示例。它们并非与 River 不兼容。[GORM 接入](https://riverqueue.com/docs/gorm)、[Bun 接入](https://riverqueue.com/docs/bun)

区别在通知路径：database/sql 接入可只轮询，也可额外配置 pgx listener 获取 LISTEN/NOTIFY；查询和事务仍走原 sql.DB。采用此方案时，需要将监听连接计入总预算。不要因为都用了 PostgreSQL，就假定不同连接上开启的事务可以自动合并。[River 驱动说明](https://riverqueue.com/docs/database-drivers)

因此，pgx + sqlc 的优势是减少适配步骤，不是独占原子入队能力。Ent 的事务桥接应在采用前做小型验证；本轮没有验证其与 River 的具体组合代码。

## 迁移和容易遗漏的行为

sqlc 不执行迁移，但可以读取 Goose SQL 迁移并忽略 Down 部分。建议以迁移为表结构来源，文件名使用定长序号或时间戳，保持字典序和执行顺序一致。[sqlc 迁移支持](https://docs.sqlc.dev/en/latest/howto/ddl.html)

GORM 提供 AutoMigrate，但这不等同于经过审查的版本化发布。项目若改用 GORM，仍建议保留 Goose；不要让应用启动时自动执行生产结构变更。[GORM 迁移](https://gorm.io/docs/migration.html)

Ent 支持自动和版本化迁移，官方生产建议偏向版本化流程，并提供 Atlas 集成；选择 Ent 后应明确谁负责结构定义、迁移生成与执行，避免双重维护。[Ent 版本化迁移](https://entgo.io/docs/versioned-migrations/)

GORM 的结构体 Updates 默认忽略零值，false 和 0 的写回要显式选字段或使用合适的 map 更新。例如把成员“启用”改为 false，不能误认为结构体中的 false 总会写入数据库。泛型 API 同样需要注意这一点。[GORM 更新规则](https://gorm.io/docs/update.html)

GORM 默认对创建、更新和删除使用事务。不能仅为追求文档中的性能提升就全局关闭；先明确业务原子性，再用等价事务语义比较性能。[GORM 性能说明](https://gorm.io/docs/performance.html)

## 4C4G 下的重点

这些工具作为 Go 库或构建期生成器使用，不要求额外常驻 ORM 服务。仅凭框架名称无法判断本机容量，也不能给出可靠的“多占多少 MB”。sqlc 生成过程可放在开发机或 CI。

优先控制返回数据量、查询次数、索引、连接总量与后台并发。PostgreSQL 的 work_mem 是查询操作层面的预算，多个排序、哈希及并发会话可能叠加，因此设置 4MB 并不意味着每个连接总共只用 4MB。[PostgreSQL 内存配置](https://www.postgresql.org/docs/current/runtime-config-resource.html)

针对本项目建议：

- 列表分页且只取所需字段；成员列表不要逐行额外查询角色造成查询次数随人数增长。
- ESI 写入分批执行并使用幂等键；大规模导入可评估 pgx COPY，涉及冲突更新时设计暂存表加合并流程，不把 COPY 当作直接 upsert。[pgx 功能](https://github.com/jackc/pgx)
- API 与后台任务共用受控连接预算，预留任务监听、迁移和维护连接。先前最大连接数 10 仍只是待验证起点。
- 对实际统计 SQL 检查 EXPLAIN，开发数据库中按需要使用 EXPLAIN ANALYZE；后者会实际执行语句。[PostgreSQL EXPLAIN](https://www.postgresql.org/docs/current/using-explain.html)

## 后续验证范围

建项后用真实 PostgreSQL 验证三类场景：带军团范围的成员分页、分批同步角色数据、业务写入和 River 入队的共同提交/回滚。若仍在 GORM 和 sqlc 之间犹豫，再以相同表结构、SQL 结果、索引、事务语义、连接上限和数据规模实现同一小功能，比较代码维护量、查询次数、P95 延迟、内存与数据库负载。

在没有这些测量前，保留现定 pgx + sqlc 是架构建议，不是性能胜负结论。本次不安装依赖，不替换技术栈。
