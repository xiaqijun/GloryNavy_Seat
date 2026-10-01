# SDE 物品与星系名称导入

> 状态索引（2026-09-23）：本指南对应的业务已上线；未交付的扩展与真实验收缺口见[待办](../backlog.md)，当前版本见[项目状态](../project-status.md)。下文带日期/Goose 的发布前记录保留当时语境，不能据此认定当前未发布；升级执行仓库最新迁移，不停在模块初始版本。

2026-09-19 双语数据补齐：业务 API 携带 `Accept-Language: zh-CN|en`，服务器按请求上下文选择展示语言，并返回 `Content-Language`、`Vary: Accept-Language`；私有接口继续 `no-store`。类型/星系名称从本地 SDE 批量解析，缺失回退另一语言、已有类型名称缓存或 ID。合同空描述摘要和服务端固定错误按语言显示；角色名、玩家备注、合同描述保持原文，不改变业务 ID、权限、价格、合同内容令牌或 ESI 缓存。无新迁移、scope 或导入要求，仅本地。详见[界面语言](../ui/language.md)。

奖励配置新增本地物品搜索：StaticDataService.SearchTypes 在当前活动 SDE 中查类型 ID 或中英文名称字面子串，2–80 字符，每批最多 30 项，排除 type 0，不请求 ESI。显示按界面语言选择（默认中文）；管理员配置的 ISK 价值不来自 SDE。

## 星系名称扩展（Goose 23，mapper 2）

当前导入 `types.jsonl` 和 `mapSolarSystems.jsonl`，不包含完整地图。星系名称经 `StaticDataService.SolarSystemNames` 批量查询，SDE 当前语言 → 另一语言 → 页面 `#ID`；无逐项 ESI 请求。已有点名/损失保存的星系 ID 不变，导入完成后即可显示名称，不必重新点名。

升级前停服务并备份开发库及配置，执行 `npm run db:migrate`，然后 `npm run sde:import -- --latest`，再启动同版服务。同 build 的 mapper 1 也会升级为 mapper 2；已有 ZIP 可复用。显式固定版本不会被自动更新解除；固定旧版时可用 `--file <official-zip> --build <build>` 显式导入，或使用 `--resume` 后等待自动更新。较新 build 不被旧 build 覆盖。

物品与星系在同一事务、同一活动版本发布，沿用租约/fence 和回退固定规则。星系文件必须存在，解压上限 64 MiB、单行 4 MiB、最多 100,000 行；校验 ID、名称、重复 ID、CRC、空集及相对活动版本超过 20% 的数量下降。`--status` 新增 `mapper_version`、`system_count`。回退到 mapper 1 的保留版本时星系名回退 ID，不混读其他版本；普通代码回退保留 Goose 23 表，不执行 Down。以下 Goose 14/15 记录描述最初的物品导入阶段。

2026-09-14 已实现 `type-names` 范围，使用官方 Tranquility JSONL ZIP 的 `_sde.jsonl` 和 `types.jsonl`。导入所有物品类型的 `zh`、`en` 名称，包括未公开类型和 ID 0；当时不导入描述、分类、星系、配方或市场价格；星系名称现已补充（见上节）。这不是完整 `core` SDE。

## 使用

升级至 Goose 15 时先停止旧 API 并备份数据库，然后在仓库根目录运行：

```sh
npm run db:migrate
npm run sde:import -- --latest
npm run start:api
```

脚本读取本机 `.env` 的 `DATABASE_URL`。`--latest` 从[官方构建元数据](https://developers.eveonline.com/static-data/tranquility/latest.jsonl)读取唯一的 `_key=sde` 记录，先比较活动版本；只有发现更高 build 才下载固定版本 JSONL ZIP 至 `SDE_WORK_DIR`（默认 `.local/sde/`）。只接受官方 HTTPS 域名的重定向；临时文件下载完成后改名。已有同名 ZIP 会复用，再由导入器校验。

也可导入已经下载的官方包，或显式回退到保留的版本：

```sh
npm run sde:import -- --file .local/sde/eve-online-static-data-3503375-jsonl.zip --build 3503375
npm run sde:import -- --activate <release-id>
```

`--activate` 会同时固定该版本并暂停自动更新，正在下载的旧任务也不能覆盖它。`release-id` 使用导入命令返回的本站版本 ID，不是 CCP build。上述 build 是本地核验示例。相同 build、ZIP SHA-256 和 mapper 版本重复导入直接返回既有版本，不重复写入，也不自动改变活动指针。重新启用既有版本请使用 `--activate`。SHA-256 用于本地一致性记录，不是官方签名。活动版本之后的新导入不得倒退 build。

## 读取优先级

合同物品按批次查活动 SDE，依次取 **SDE 中文 → SDE 英文 → 已缓存 ESI 名称 → `#ID`**。仅在整个类型缺失时使用 ESI 名称缓存；不会将英文伪存为中文。API 的 `name_language` 为 `zh`、`en` 或空字符串。名称解析集中在 `StaticDataService.TypeNames`，合同通过注入接口复用；后续资产、配装等业务使用同一服务，不直接导入 eve store，也不各自建立名称缓存。

物品列表不逐项请求 ESI、不触发合同重同步。非物品的公开角色、军团、空间站仍沿用原来的批量名称补充；合同中的玩家建筑通过带角色授权的 ESI 接口解析并隔离缓存，失败保留 ID，见[合同地点名称](contracts.zh-CN.md#地点名称2026-09-17)。中文显示不需要重新授权或重新同步合同。新版本提交后下次请求直接生效，无额外 SDE 进程缓存。

## 数据与失败恢复

Goose 13 将旧 ESI 名称缓存标为 `en` 并按语言隔离；Goose 14 新增 `eve_sde_name_releases`、`eve_sde_type_names` 和 `eve_sde_active_names`，归 eve 私有 store 管理。导入器通过 eve 服务暴露给运维 CLI，没有新增后台管理权限或 HTTP 导入入口。Goose 15 新增 `eve_sde_update_state`，保存检查时间、活动租约、fence、固定版本状态、最近结果与失败次数。

ZIP 上限 512 MiB，元数据 1 MiB，types 解压数据 1 GiB，单行 4 MiB，名称 4096 字节，最多一百万个类型。当前仅读取元数据、物品和星系三个白名单文件，不解压到文件系统；检查路径、重复文件、构建号、CRC、JSON、必需字段和唯一类型 ID。未知字段兼容，空数据或比活动版本少超过 20% 的数据拒绝发布。

下载在事务外完成；导入使用事务级 advisory lock 串行化，1000 行一批 COPY，在同一事务中提交版本、数据和活动指针。旧数据在导入期间继续可读。任一步失败回滚全部候选写入并由 CLI 返回错误；当前版本保留，不持久化半成品 release；自动更新的最近失败分类与退避时间另存更新状态，手动导入错误由命令输出。提交成功的版本均可再次激活，旧版本暂不自动清理。

自动更新和最近状态已实现；完整失败历史面板、排队补充缺失类型、其余 SDE 数据集和跨报表版本固定仍待实现。不要把[早期完整 SDE 方案](eve-integration.zh-CN.md#10-sde-初次导入流程)中的环境变量、调度、profile 或发布状态机当作已经实现的命令。

## 自动更新与运维状态

借鉴 SeAT 的独立 SDE 调度、业务模型统一读取及按版本避免重复导入；保留本项目中文优先、原子切换和显式回退。SeAT 参考为 `eveapi` 提交 `990a0a2`：[合同模型](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Models/Contracts/ContractItem.php)、[默认月度计划](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/database/seeders/ScheduleSeeder.php)、[SDE 版本检查](https://github.com/eveseat/eveapi/blob/990a0a29649d0701f35605739c2e9b823275a13a/src/Commands/Eve/Update/Sde.php)。本项目默认周期为 6 小时，不声称与 SeAT 参数相同。

| 配置 | 默认 | 行为 |
| --- | --- | --- |
| `SDE_AUTO_UPDATE` | `true` | eve 模块启动后启用公共 SDE 更新，无需 SSO 凭据；设为 false 并重启可关闭 |
| `SDE_CHECK_INTERVAL` | `6h` | 成功检查后的间隔，接受 1h–168h |
| `SDE_WORK_DIR` | `.local/sde` | 官方 ZIP 缓存目录；生产配置可写且持久化的目录 |

复用 River 的 30 秒到期扫描；独立 `eve_sde` 队列单 worker，活动任务唯一约束与数据库租约防止跨实例重复执行。队列任务 `eve.sde-update.v1` 不含凭据，也不按角色重复运行。新构建才下载和导入，当前或更旧构建仅记录 `unchanged`。到期时间在 PostgreSQL 中，重启不会重置六小时周期；无 SSO 时也可运行，ESI 业务就绪状态仍独立判断。

工作预算 15 分钟，River 超时 16 分钟，租约 20 分钟；中断后由 River 恢复，到期扫描继续兜底。自动发布在事务内核对 fence、租约、固定标记及当前 build，过期任务不能覆盖较新状态。失败按 5、10、20 分钟逐步退避，上限 6 小时，成功清零；River 重试不得提前越过数据库中的到期时间。SDE 的网络或导入失败不改变已发布版本，独立队列避免占用角色／合同 worker。

```sh
npm run sde:import -- --status
npm run sde:import -- --resume
```

`--status` 输出 JSON：活动版本／build／SHA-256／类型数、pinned、最近检查／成功时间、最近结果、错误分类、失败次数和下次检查时间。错误分类为 `metadata_failed`、`download_failed`、`import_failed`、`database_failed`，不包含令牌或私有响应。`--resume` 解除手动版本固定并使下次检查立即到期；实际执行仍要求运行中的服务启用 `SDE_AUTO_UPDATE`。`--latest` 是显式手动操作，不解除固定标记；同 build 无需下载校验整个 ZIP，需要检查本地 ZIP 时使用 `--file --build`。

缓存 ZIP 校验失败时，先移走对应问题 ZIP，再重新更新；失败重试会复用已有 ZIP，不擅自覆盖运维提供的文件。自动状态目前通过命令读取，尚未加入 `/sync` 页面。

## 验证

本地已核验官方 build 3503375：52,999 个类型，52,585 个中文名，414 个回退英文。合同中 771 种物品全部命中，767 种有中文。重复导入返回同一个版本，未重建数据。

数据库回归覆盖中文优先、英文及 ESI 缓存回退、缺失 ID、无逐项网络请求、重复导入、回退、ID 0、未知字段，以及重复类型、错误 build、旧 build、空数据、数量异常、非法 JSON、长行、ZIP 重复条目与路径穿越拒绝；失败后活动版本和已发布版本数保持不变。

官方格式说明：[Static Data](https://developers.eveonline.com/docs/services/static-data/)。英文版本：[SDE type names](sde-names.en.md)。

自动更新回归另覆盖：重复扫描去重、同版本不下载、持久化到期时间、失败退避与最后成功时间保留、已保留版本重新启用、下载中回退、旧 fence 拒绝发布、无 SSO 的真实 River 启停及元数据重复记录拒绝。

本地首轮自动检查于 2026-09-14 15:54:11（UTC+8）返回 `unchanged`，保持 build 3503375／release 1，下次检查时间 21:54:11；API 就绪通过。新构建切换和故障场景使用测试夹具验证。

## 配装参考数据边界

当前配装页不再提供属性模拟，名称仍复用 StaticDataService。Go 服务嵌入官方 build 3503375 的轻量类型/槽位/技能前置参考，用于 EFT 与技能生成，不代表数据库已导入全量 Dogma。名称自动更新不替换固定参考；生成步骤见[配装指南](fittings.zh-CN.md)。旧模拟依赖保留为历史源码。

## 技能类别目录

技能页面名称仍优先 StaticDataService；技能类别/组名称及服务端白名单来自固定官方 SDE 3503375 的轻量嵌入目录。该目录不由数据库自动名称更新替换，升级步骤见[技能管理](skills.zh-CN.md)。
