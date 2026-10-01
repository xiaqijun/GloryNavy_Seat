# ISK 钱包

> 状态索引（2026-09-23）：本指南对应的业务已上线；未交付的扩展与真实验收缺口见[待办](../backlog.md)，当前版本见[项目状态](../project-status.md)。下文带日期/Goose 的发布前记录保留当时语境，不能据此认定当前未发布；升级执行仓库最新迁移，不停在模块初始版本。

2026-09-20 生产术语修正（前端 `v0.1.0-terminology-20260920`）：162 项流水名称现从官方 SDE accountingEntryTypes 生成，中英文均使用原名；例如 ESS 为“事件监测装置保证金支付 / ESS Escrow Payment”。关联对象 10 项使用客户端标签，2 项保留英文技术名称，不再猜译。来源及更新方法见[术语指南](eve-terminology.zh-CN.md)。不改变 ESI 代码、筛选值或历史数据，发布验收见[项目状态](../project-status.md)。

2026-09-19 双语数据补齐：业务 API 携带 `Accept-Language: zh-CN|en`，服务器按请求上下文选择展示语言，并返回 `Content-Language`、`Vary: Accept-Language`；私有接口继续 `no-store`。类型/星系名称从本地 SDE 批量解析，缺失回退另一语言、已有类型名称缓存或 ID。合同空描述摘要和服务端固定错误按语言显示；角色名、玩家备注、合同描述保持原文，不改变业务 ID、权限、价格、合同内容令牌或 ESI 缓存。无新迁移、scope 或导入要求，仅本地。详见[界面语言](../ui/language.md)。

2026-09-19：前端支持中英文界面，词表对齐 [ESI 官方 OpenAPI（兼容日期 2026-08-18）](https://esi.evetech.net/meta/openapi.json?compatibility_date=2026-08-18) 的 162 种 `ref_type` 与 12 种 `context_id_type`。筛选/数据库保留官方原值，未知值回显原文。列表展示类型和玩家备注，详情保留完整原始说明、原始类型与上下文；不翻译玩家内容。语言偏好及 SDE 名称边界见[界面语言](../ui/language.md)。不改变 API、权限、同步窗口或迁移。

2026-09-16 本地新增，Goose 33/34。`/wallet` 提供个人钱包及军团分部钱包的余额、收支流水与市场交易；不执行游戏付款。**后续补损交付依据合同核对，钱包流水不自动改变补损状态。** 果壳币继续由 exchange 管理。

## 启用与权限

先备份数据库和配置，执行 `npm run db:migrate`（包含 Goose 与 River），向现有 `MODULES` 追加 `wallet`，保留 `identity,eve,access`，重新构建前后端并重启 API。无需新增 OAuth scope：既有登录方案已包含个人钱包、军团钱包和军团分部读取权限；旧凭据缺失时需要本人重新授权。

- 个人钱包：本人有效绑定角色；当前站点管理员可从成员页面进入其他成员钱包。每次请求重查管理员标志、有效绑定、游戏所有者、凭据状态及 scope。管理同步或权限配置能力不等于查看所有成员钱包。
- 军团钱包：同时检查 `corporation.journal`（流水）或 `corporation.transaction`（市场交易）以及 `corporation.wallet_first_division` 至 `corporation.wallet_seventh_division` 对应分部能力。余额和分部名称只提供已授权分部。权限管理目录新增这些已实现能力，资产分部仍隐藏。
- 保持既有 SeAT 映射：Accountant 提供流水/交易能力，Account_Take_1…7 提供对应分部；CEO、Director 与当前站点管理员可读已解析军团范围。Junior_Accountant 能作为 ESI 数据源，不自动获得本站钱包查看权。本站授权与 ESI 抓取职务是两个检查层。
- 角色转移到其他游戏所有者、解绑、撤权后不能借历史缓存绕过读取校验。

## 官方端点与同步

依据 [ESI 官方 OpenAPI](https://esi.evetech.net/meta/openapi.json?compatibility_date=2026-08-18)，保持项目兼容日期。下表时间是接口缓存基准，实际调度以响应 `Expires` 和共享客户端的限流结果为准。

| 资源 | GET 路径 | 缓存基准 | 翻页 |
| --- | --- | --- | --- |
| 个人余额 | `/characters/{id}/wallet` | 300 秒 | 无 |
| 个人流水 | `/characters/{id}/wallet/journal` | 3600 秒 | `page` / `X-Pages` |
| 个人市场交易 | `/characters/{id}/wallet/transactions` | 3600 秒 | `from_id`，跳过重复边界，直到没有更早记录 |
| 军团 7 分部余额 | `/corporations/{id}/wallets` | 300 秒 | 无 |
| 军团分部流水 | `/corporations/{id}/wallets/{division}/journal` | 3600 秒 | 逐分部、`page` / `X-Pages` |
| 军团分部市场交易 | `/corporations/{id}/wallets/{division}/transactions` | 3600 秒 | 逐分部、`from_id` |
| 自定义分部名称 | `/corporations/{id}/divisions` | 3600 秒 | 无；仅返回有自定义名称的项 |

个人 scope 为 `esi-wallet.read_character_wallet.v1`；军团为 `esi-wallet.read_corporation_wallets.v1`，数据源须有新鲜的 Accountant / Junior_Accountant / Director / CEO 职务事实。分部名称使用 `esi-corporations.read_divisions.v1`，数据源须为 Director / CEO；无此数据时界面使用“分部 N”。不抓取无职务角色的军团钱包。

同步资源 ID：`wallet_balance`、`wallet_journal`、`wallet_transactions`、`corporation_wallet_balance`、`corporation_wallet_journal`、`corporation_wallet_transactions`、`corporation_wallet_divisions`。通过共享 River `eve.wallet-resource.v1` 作业、ESI 客户端、缓存和令牌桶执行，不另建轮询器。军团按“军团＋资源”选择一个可用授权；凭据阻塞后其他合格角色可接替。

每次作业处理一页，分页、分部、授权代次和最早缓存到期时间持久化，提交与目标租约/fence 检查同事务。发布军团页前再次核查授权职务及军团归属；旧授权、旧租约不能覆盖新数据。分页数变化时重新扫描，途中已保存历史不清空。

ESI 流水端点仅提供最近 30 天；同步按记录 ID＋内容指纹去重更新，同号不同内容保留各自记录（不自动把这些版本相加），保留已采集的更早记录，不能补回首次接入前 ESI 已不提供的历史。交易按照官方返回窗口取完，不声称能获取全部历史。原始观察按来源凭据与游戏所有者隔离：删除凭据会级联删除该来源的观察；撤 scope/所有者变化使该来源记录不可读，后续合格来源可重新采集仍在 ESI 窗口内的数据。因此这不是永久财务档案。

## 展示与契约

`GET /api/v1/wallet/context?member=...` 返回可读归属、分部与数据类别；指定其他成员仅管理员可用。`GET /api/v1/wallet/records` 参数见 [OpenAPI](../../api/openapi.yaml)：归属、分部、类别必选；流水支持类型、收支、参与方 ID、说明/备注/记录编号、时间；交易支持买卖方向、参与方 ID、交易编号、时间。每页 50 条，以 `before=记录ID.内容指纹` 复合游标翻页（交易为记录 ID）。日期 `from` 包含、`until` 不包含，页面结束日期覆盖当地整天。

工作台个人钱包使用 `GET /api/v1/wallet/personal-corporation-summary?corporation_id=...&from=...`，汇总当前军团成员中有效绑定角色的最新余额及本月收支。服务端仅纳入仍属于该军团、当前凭据所有者匹配、凭据未阻塞且具备 `esi-wallet.read_character_wallet.v1` 的角色；请求还要求当前站点管理员的 `access.members.read`。这是军团范围汇总，不接收任意成员参数，也不返回逐角色明细。`from` 必须在过去 45 天内；接口只读本地快照，同一流水编号多份历史观察只取最新版本，缺失余额返回 `null`，金额和军团 ID 使用十进制字符串。工作台的“个人钱包”因此代表全体成员可读角色的合计，个人钱包详情页仍使用原有当前账号/管理员成员读取接口。

军团运营面板使用 `GET /api/v1/wallet/corporation-summary?owner_id=...&from=...` 汇总当前管理员可读的军团钱包分部，并使用上面的 `personal-corporation-summary` 汇总成员个人钱包，返回最新余额、本月收入和本月支出。服务端分别重查军团钱包能力、成员数据权限与有效角色范围；缺少权限、快照或同步时不显示为零。两个接口只读本地快照，不创建付款，也不绕过 ESI 缓存；无新迁移和 scope。

ID 与金额保持十进制字符串，不经 JavaScript 浮点计算。缺失金额显示“—”，不转成零。ESI `first_party_id` / `second_party_id` 含义随类型变化，显示“参与方”，不统一冒充付款/收款人。未知官方类型保留原值；详细 `description`、`reason`、上下文、关联流水可在统一弹窗读取，`journal_ref_id=-1` 不当作有效关联。静态类型名走 SDE；参与方名称优先缓存，必要时通过共享 ESI 客户端批量解析公共名称，失败回退 ID。

页面刷新读取本地快照，不绕过 ESI 缓存；后台更新后替换同一钱包数据，不以“待更新”遮盖历史。管理员可在同步管理查看 7 类资源的执行结果及重试。

## 模块与回退

EVE 私有 store 保存 `eve_wallet_observations` / `eve_wallet_cursors`；wallet 只负责读 API 与对象权限，经宿主注入 EVE 业务接口和 StaticDataService，不导入其他模块 store。数据按角色/军团归属，没有新增账号级币账，账号合并沿用角色归属迁移，不重复移动 ISK 或改变记录编号。

停用 `wallet` 后不再调度其资源；已存在作业暂停，不删除数据。回退到不认识该 kind 的旧程序前，停止 worker、取消 `eve.wallet-resource.v1` 未结束作业并阻塞钱包目标；回退 schema 前确认备份，Goose 34 down 遇到同号多版本会拒绝折叠；Goose 33 down 删除钱包数据。不要在生产直接套用本地测试启动脚本。

验证结果及真实联调范围见[项目状态](../project-status.md)，页面布局见[钱包页面](../ui/wallet.md)。

无游戏职务的钱包资源每 5 分钟重新检查，状态和运行结果均为延期，不增加失败次数、不发送钱包请求；修复前的历史运行结果保留。

运营面板补充：成员收入变化使用 `GET /api/v1/wallet/personal-corporation-income-trend`，按当前军团有效绑定角色的本地钱包快照聚合 UTC 月度收入、支出和有收入成员数，供近六个月趋势展示。服务端不发起 ESI 请求，重复同步只取最新观察。

运营面板财务卡片使用 `GET /api/v1/wallet/corporation-finance-trend?owner_id=...&from=...&until=...` 获取授权军团分部的月度收入、支出、税收和净变化。前端只使用军团钱包余额作为总额，税收取当前 UTC 月，并分别展示近六个月钱包净变化趋势和税收趋势；金额保持十进制字符串，接口只读本地快照并复用军团钱包权限检查。
