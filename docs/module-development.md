# 新增模块开发约定

## 2026-09-23 元数据收敛（已发布）

匿名首页与 `/login` 不再请求模块目录；`GET /api/v1/eve/login-status` 仅返回 configured。`/api/v1/modules` 与完整 `/api/v1/eve/status` 要求有效会话，`/api/v1/system/status` 只允许当前站点管理员。`/health/ready` 只返回 ready/503，不输出版本、数据库及 schema；liveness 不变。会话校验在读取目录之前，资料未补齐仍可进入账号页。前后端需一起发布，部署探测已改为检查匿名 401 与最小登录投影。

公开军团首页由必需的 system 模块声明 `system.landing`（`/`），宿主仅为这一已审查页面绕过业务目录加载；不建立通用“公开模块”开关，不绕过其他页面/业务 API 的原鉴权。业务工作台为 `/workspace`。详情见 [首页](ui/homepage.md)。

## 接入审批中心来源

approval 通过宿主注入 reviewqueue.Source，来源提供 Access、Query、People；SQL 必须留在来源私有 store。Query 在分页/计数前过滤授权对象，时间、来源、ID 必须稳定排序；不可把前端隐藏当权限或在列表读取中访问 ESI。源写 API 继续检查版本、幂等、当前权限和对象归属，原单及审计为唯一事实源。新来源不得自行复制币账。当前例子见 internal/app/approval.go 与[指南](integrations/approval.zh-CN.md)。

自动合同结算编号使用 Goose 41 的类型-UTC日期-UUID 规则，创建时由数据库生成并持久化。不要在前端、读取接口、重试或自动核对时重新生成；不要替换已有编号或把 ESI 数字合同 ID 改成业务编号。详见福利/兑换中英文指南。

成长福利跨模块校验：宿主通过 fittings.RequiredSkills 和 skills.CheckRequirements 合并当前配装先修/技能方案要求，welfare.GrowthCheck 只读本人观测。申请事务使用宿主注入 GuardGrowth → 各模块 GuardLibraryVersion/GuardPlanVersion 的共享行锁复核版本，SQL 保持私有，不在发布事务内请求 ESI。福利账号合并同步核对同船型跨项目占用。见[成长福利](integrations/welfare.zh-CN.md)。

新增合同交付消费者必须经宿主注入 EVE ClaimDeliveryTx 参与跨模块唯一占用，不能只依赖模块私有去重。兑换核对任务只传 order_id，先身份/活跃账号锁再钱包/订单锁，读本地证据，禁止在写事务抓 ESI。回退需取消对应 River 新 kind。见[兑换指南](integrations/exchange.zh-CN.md)。

页面可用 `administratorOnly: true` 控制当前站点管理员的导航入口（奖励库）；它仅影响可见性，不是可委派能力。服务端仍使用受保护的 exchange.self 入口并逐次检查管理员，不能以菜单隐藏代替对象鉴权。

固定前端文案使用 `lib/i18n.ts` 的 `msg` 与统一英文词表；协议值、权限键和用户输入不翻译。日期/数字格式使用 `getLocale()`，业务时区保持独立。见[界面语言约定](ui/language.md)。

前端 PageDefinition 可声明 navigationGroup（operations/finance/benefits/administration），由宿主先按模块/权限过滤再分组。空组隐藏，当前路由组展开；分组只改变展示，不替代路由和对象鉴权。未声明分组的导航页为直达入口。见[多级导航记录](ui/market.md)。

## 补损交付合同边界（含历史兼容）

Goose 35：welfare 持有确认快照、全局合同预留及持久检查时钟，宿主注入 EVE 的 DeliveryContracts / DeliveryContractTx。EVE 复用既有对象鉴权，读取本地合同及物品，提交事务锁合同/明细避免证据并发变化；网络抓取仍由 EVE 原同步器负责。共享 River 新增 welfare 队列及 delivery-scan/check.v1，旧手动关联保留历史证据；当前批准后自动匹配与核验，后台复核发放者和审核者当前权限。账号合并沿用福利原参与方，不复制来源权限。详见[福利指南](integrations/welfare.zh-CN.md)。


## 钱包示例

wallet 已实现个人与军团分部 ISK 读取，EVE 拥有同步 SQL；宿主注入读取及权限边界。新增 source 不可绕过 current binding、游戏所有者与军团职务/fence 校验。现已开放 journal、transaction、wallet 1…7 分部能力；未实现的资产分部仍不开放。详细约定见 [钱包指南](integrations/wallet.zh-CN.md) / [English](integrations/wallet.en.md)。


## 福利与考勤服务协作

宿主 `wireWelfareAttendance` 将 attendance 的 `ConfirmedReimbursementLosses`、`LockReimbursementLoss` 注入 welfare 的 `AttendanceLosses`、`GuardAttendanceLoss`。列表先鉴权原始角色损失，再按当前绑定账号批量读取同军团的已确认出勤损失；写入按原单账号检查。福利不导入考勤私有 store。军团补损提交/批准在绑定、凭据与账号锁之后、福利锁之前，对活动、出勤条目、已确认损失取共享行锁并重查，避免确认状态被并发撤销。考勤 UI 未启用不影响校验已有记录；无服务或无关联拒绝军团补损，PVP不受此门槛限制。

Goose 32：welfare 的 Losses/GuardLoss 由宿主接到 EVE 本地报告服务，原始发现与明细不归福利。写入前按角色锁 → EVE 凭据锁 → 活跃账号锁 → 福利发布锁重核证据，ESI 网络读取仍只在后台发布事务之外。原始数据按角色凭据生命周期，申请保存独立快照，不增加账号级数据合并参与方。

Goose 31 welfare 示例：`identity.LockActiveAccounts` 通过宿主注入，在角色锁之后、模块发布锁和钱包锁之前取得排序账号共享锁，阻止合并后的迟到写入。福利主动发币用专用 `WelfareGrantTx`，确认单据是金额来源，不经过 PAP 比例或通用客户端加币入口。模块合并冲突可实现 `MergeBlockReason() string` 返回固定、可公开的解释，由 identity 返回 409；不传敏感材料或内部错误文本。停用模块仍参与既有数据合并。

账号合并（Goose 30）：新增账号级数据必须明确是否迁移、原归属追溯和停用账号写入边界。通过模块业务服务暴露 `MergeAccountTx(ctx, tx, source, target, apply)`，由宿主登记到 identity；预览返回确定性摘要/指纹，提交同事务迁移，SQL 仍归私有 store。停用 UI 不代表可遗漏已有数据。保存原操作者/金额/时间，账户归属变更用 `original_account_id` 追溯；旧请求写入使用 identity 活跃账号检查契约。详见[合并指南](integrations/account-merge.zh-CN.md)。

Goose 26 的 CoinConversion 仅接收来源模块提供的已发权益，GET预览不写数据、POST需管理员与原报价。来源回调 Previous 必须是变更前积分，不能固定为0；用于避免切换自动模式时补兑未兑换历史。各来源模式与首次实际兑换比例由 exchange 管理。

Goose 25 兑换边界：exchange 私有表保存币权益、流水、奖励与订单；来源模块提供稳定引用、原账号、旧/新数量，由宿主注入 ReconcileTx 业务回调，必须与源业务同事务。先排序锁来源账号，再排序锁币账号，之后锁比例/权益；领取按绑定、币账号、估值、奖励顺序。仅 PAP 已注册；未知来源不开放，配置/发放要求站点管理员。详见[兑换指南](integrations/exchange.zh-CN.md)。

业务模块现可由宿主向 NewSync 传入 platform/jobs.Extension，在 River 创建前注册 worker/周期任务并注入同一客户端。attendance 保存事务性任务行，由独立扫描器恢复，后台载荷只含 ID。停用模块仍注册处理器以休眠旧作业，但不注册新的周期扫描；旧二进制回退须先取消它不认识的作业。不是运行时插件安装或动态代码执行。

本项目扩展的是随应用编译发布的可信模块。先阅读 [架构边界](architecture.md) 和 [UI 规则](ui-design-rules.md)。当前可运行范例为 `internal/modules/system` 和 `web/src/modules/system`。

## 后端

1. 确定唯一小写 ID，例如 `community`；包放在 `internal/modules/<id>`。目录分离按实际职责需要进行，不为未实现的能力创建空仓储或空 worker。
2. 编写业务服务；依赖通过构造函数传入。跨模块只依赖需要的业务服务接口，模块构造阶段不做网络访问或启动 goroutine。
3. 返回 `module.Definition`，声明 Manifest、Requires、Permissions 和 Routes。路径相对本模块，例如 `/profile` 最终是 `/api/v1/community/profile`。当前路径支持小写字面段和 `{id}` 参数，不支持通配符或自定义正则；根操作使用明确的资源路径。
4. `Permission` 使用本模块前缀，例如 `community.profile.read`；`Public` 默认 false。未启用 identity 时受保护接口不能启动。已启用时宿主验证会话并委托 access；新能力必须登记策略，对象接口必须解析服务端目标再做范围检查。不要用 Public 或仅 access.self 绕过业务授权；参考 access 的 summary handler。
5. 在 `internal/app/app.go` 明确创建并注册模块；在 `MODULES` 启动清单加入其 ID 和依赖。先校验启用关系，再为未来需要后台资源的模块启动生命周期；当前 Definition 本身只描述路由，不会自动启动任务。
6. 查询放入模块内的 `internal/store/queries`，sqlc 输出到同一个私有 store。根 `migrations` 新增带模块归属注释的 Goose 迁移，并在 `sqlc.yaml` 增加对应生成块。不得编辑已生成 `.sql.go`，不得把草稿迁移加入发布列表。
7. 更新 `api/openapi.yaml`，覆盖服务规则、权限拒绝、对象范围、依赖错误及实际数据库行为。`npm run db:generate` 后提交生成文件；CI 检查整个 `internal/modules` 的生成一致性。

宿主统一提供请求编号、JSON 响应、日志、数据库池与鉴权包装。HTTP handler 使用 `httpapi.Respond/Failure`，可通过 `httpapi.RequestID` 关联日志；敏感参数、凭据和令牌不能进入日志。

## 前端

弹窗复用 `components/ui/dialog.tsx` 的 `Modal` / `ConfirmDialog`；特殊认证流程可保留 Radix 控制结构但复用共享外层 CSS。不要再新增模块私有的遮罩、定位与表面样式，不使用 `window.confirm` 执行业务确认。长表单使用独立滚动正文与 `footer` 操作区，尺寸和状态要求见[UI 规范](ui-design-rules.md#弹窗一致性)。

1. 放入 `web/src/modules/<id>`，通过 `index.ts` 导出 `FrontendModule`，ID 与后端一致，apiVersion 匹配宿主契约。
2. 页面 ID 使用 `<模块 ID>.<页面名>`；声明路径、中文导航名、Lucide 图标和 `load: () => import(...)`。当前注册表支持固定页面路径；需要详情页动态路由时再扩展路径校验及冲突测试。
3. 在 `web/src/app/modules.ts` 明确导入。页面和导航共用注册表，页面默认进入导航，`navigation: false` 可声明仅通过操作入口访问的页面。默认采用工作台布局；登录等独立页面可声明 `layout: "standalone"`，由宿主隐藏侧栏和顶栏。布局与导航配置不代替鉴权；权限页面可声明 `permission: "access.manage"`，宿主依据后端能力过滤导航，页面同时实施访问门禁；新增能力需扩展明确契约，不能仅隐藏入口。
4. 数据请求使用 `lib/http.ts` 的统一响应处理，再由模块校验业务数据。TanStack Query 缓存键以模块 ID 开头，例如 `["community", "profile", userId]`；当前登录通过整页跳转初始化新缓存，退出清空缓存；后续小号切换需补齐独立失效规则。
5. 复用共享 UI、主题和图标操作；不得从 API 下载并执行插件 JavaScript。系统模块为必需项，其他可选模块由后端目录选择；前端可见性不能代替服务端权限。

## 发布与停用

页面注册生成共享 `preload()`，宿主用于当前地址及可见导航的代码预载；模块顶层不得有私有数据请求或业务副作用，停用模块即使匹配本地地址而下载代码也不得被渲染。预载失败由实际导航的错误边界提供恢复。业务预取在各模块内部实现，必须先确认登录、保留用户／对象／筛选查询键，复用已有对象鉴权 API；不能为了并行加载跳过访问范围确认或把其他对象的缓存作为占位数据。

- 新代码构建、迁移、API 和前端应作为兼容版本一起发布；接口做破坏性变更时明确契约版本和升级步骤。静态资源缺失有页面错误边界及刷新入口，不能代替资源发布一致性。
- `MODULES` 是部署配置，重启后生效，没有运行时安装和管理界面。删除配置项不会删除表、反向执行迁移或卸载 Go 包。
- 注册表拒绝未知/重复模块、缺失依赖、依赖环、API 不兼容、路由冲突及缺少鉴权的受保护接口。必需 system 被停用也拒绝启动。
- 引入后台任务时必须补齐启用顺序、停止顺序、积压任务和迁移兼容策略；不要仅增加菜单就声称支持完整插件生命周期。

验证命令：`npm run check`、`npm run db:generate`、`npm --prefix web run test:e2e`。真实数据库测试需设置开发用 `TEST_DATABASE_URL`；迁移测试仅创建并删除自己的随机 schema。

模块交付同时更新[项目状态](project-status.md)、[变更记录](../CHANGELOG.md)及受影响的接口/配置说明；具体维护范围见[贡献约定](../CONTRIBUTING.md)。新增文档加入[文档目录](README.md)。

- 权限目录仅开放已完成的对应业务能力。新增功能时同步登记 `ManageableCatalog` 和页面入口；未开发的军团业务、钱包或分部能力不得提前放入配置页面。底层历史权限兼容不等于开放前端配置。

## 后台资源扩展

ESI 资源统一依赖 `eve.ESIService.Request`，使用方可声明窄接口，由宿主注入 `AuthorizationService.ESI()`。请求只传角色 ID、授权代次与所需 scopes，不获取原始令牌或另写 HTTP/缓存/刷新逻辑。调用方仍负责业务对象和有效绑定检查，网络获取与带 fence 的数据发布分离；接口及观测口径见[接入指南](integrations/esi-client.zh-CN.md)。

当前 River 运行时在 `internal/platform/jobs`，EVE 资源调度与 SQL 在 eve 模块，宿主负责启停。新增资源应登记版本化处理器、目标/数据表、scope 与缓存策略，并验证事务入队、重复执行、代次失效和限流恢复。通用事件总线与运行时插件安装仍未实现。详见[同步运行指南](integrations/esi-sync.zh-CN.md)。

## 静态数据复用

需要物品名称的模块在使用方定义 `TypeNames(context.Context, []int64) (map[int64]eve.StaticTypeName, error)` 窄接口，由宿主注入 `eve.StaticDataService`。可参考合同 handler 的 `StaticData` 字段；中文／英文／ESI 缓存回退在服务内统一完成，页面不逐项调用 ESI。新数据集只有对应业务需要时才添加自己的映射与查询，保持下载、导入校验、版本发布、自动更新时钟的职责分离；当前有 type-names 和 solar-system-names（`SolarSystemNames` 批量读取），尚无动态 profile 注册或完整 core 数据。运行说明见[SDE 名称](integrations/sde-names.zh-CN.md)。

## 管理员成员数据读取

新增已交付的数据功能应支持站点管理员读取成员数据，使用 access.IsAdministrator 或对应已知读策略，并检查目标存在、有效绑定及范围。成员页经宿主注入 MemberData 展示 DTO，不跨模块导入 store。管理员专用 access.members.read 不可角色授予，因此不放入 ManageableCatalog；新的可委托业务能力仍随功能登记。GET 读取与 POST/PUT/DELETE 和 SSO 账号操作独立；参照合同 LookupOwner 和 SyncHTTP.CanRead／Owns。

ESI 限流观测统一由 `internal/esiclient` 记录；新增资源不得另算或按状态码伪造实际消耗。复用 Request 即接入桶/接口聚合，不向观测中传入完整查询、正文或凭据。桶查询继续只使用 `eve.sync.manage`，不能代替业务对象权限。

新增 ESI 路由复用 esiclient 的官方目录与逐笔预算，不在业务 worker 内重复计费或硬编码 15 分钟。目录更新使用 `node scripts/update-esi-catalog.mjs` 并审查兼容日期/分组差异；未声明新桶的路由仍遵守公共错误保护。读取 [Goose 20 算法与升级约束](integrations/esi-client.zh-CN.md) 后再修改并发、窗口或缓存行为。

attendance 是包含业务写入、EVE 网关与统计的扩展示例。`identity.Bindings` 支持按排序锁定角色后读取归属；EVE `OnlineDataTx` 使用调用者的事务查询本模块 store，不泄露 SQL 类型。新增资源若在旧版派发器中不存在，回退必须先阻塞其目标并取消队列作业；参见[考勤升级](integrations/attendance.zh-CN.md#启用升级与回退)。

PAP 是 attendance 内部业务，复用 corporation.attendance 范围；余额与差额流水、事件版本和审计必须同事务发布，使用事件锁和请求键保证幂等。跨模块不直接写积分表，账号归属使用出勤历史快照。

新增 fittings 模块示例：宿主显式注册后端/前端，已知自用能力 fittings.self 受保护，成员管理员读取与草稿写权限分离；fittings/skills 资源统一通过 eve 的 Request、generation/fence 和 River 派发，禁用开关同时约束派发与执行。见[配装指南](integrations/fittings.zh-CN.md)。

## skills 资源复用示例

技能管理与配装通过宿主复用 EVE 技能服务，不导入对方 store 或前端计算包。MODULES 任一启用 fittings/skills 即调度唯一 skills 目标，仅 skills 启用 skillqueue。新增的 corporation.skills 已对应方案维护/要求检查业务；skills.self 是入口能力，业务对象校验不可省略。Goose 28 及 API 见[技能管理](integrations/skills.zh-CN.md)。

军团配装库升级为 Goose 29：库、审计和游戏写幂等记录归 fittings；游戏写经 eve 的受限 Mutation 请求，非缓存调用。技能前置计算使用本模块静态参考，方案落库仍走 skills 服务/API。通过宿主提供军团可见范围、本人绑定与当前管理员，不开放新的未实现权限。

## 服务端语言与证据边界

业务 API 统一经 `apiFetch` 发送界面语言；服务端使用 `internal/platform/locale` 请求上下文，后台任务默认中文。语言只改变展示，不传给 ESI 抓取或共享缓存键。新增名称消费者经注入 StaticDataService 批量读取并保留 context。固定消息加入前端词表后运行 `node scripts/backend-messages.mjs`，用 `--check` 校验 Go 消息子集；仅对服务端固定文案使用 Message，不翻译用户输入。技能/舰船的 group_english 来自同一官方 SDE 构建，由参考目录生成脚本维护。

福利列表/详情只投影已授权快照中的类型、星系名称及固定核价提示；数据库证据和审计不改写，未知字段与精确数值保留，名称查询失败则保留原名。业务规则仍使用稳定 ID/代码，不使用翻译后的名称判断资格。
