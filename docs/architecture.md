# 模块架构与扩展边界

## 2026-09-23 元数据收敛（已发布）

匿名首页与 `/login` 不再请求模块目录；`GET /api/v1/eve/login-status` 仅返回 configured。`/api/v1/modules` 与完整 `/api/v1/eve/status` 要求有效会话，`/api/v1/system/status` 只允许当前站点管理员。`/health/ready` 只返回 ready/503，不输出版本、数据库及 schema；liveness 不变。会话校验在读取目录之前，资料未补齐仍可进入账号页。前后端需一起发布，部署探测已改为检查匿名 401 与最小登录投影。

更新：2026-09-23。当前结构以本页业务边界及专项指南为准；带 Goose 编号的段落注明首次引入关系，不表示当前生产仍停留在该版本。当前交付与待做事项见[状态](project-status.md) / [待办](backlog.md)。

2026-09-23 已发布首页活动投影：`eve.PublicActivityService` 仅公开固定 Glory Navy 的战绩与在线角色汇总，宿主注入当前 identity 绑定，EVE 私有 SQL 校验军团/owner/代次和采样；不跨 store，不公开明细，不改变其他业务鉴权。zKillboard 为独立公共上游，固定 URL/缓存/超时，不复用角色凭据。详见[公开资料指南](integrations/public-corporation.zh-CN.md)。

PAP 月度最低要求由 attendance 私有配置与审计表持有（Goose 43），使用宿主注入的 Administrator 判断写权限。全站账号级规则与个人 PAP 流水分离，无需账号合并迁移；前端复用全量月报和独立版本化配置接口。见[考勤指南](integrations/attendance.zh-CN.md)。

## 审批中心：只读聚合与统一处理入口

新增 approval 模块及平台 reviewqueue 契约，宿主按实际启用的 welfare/exchange 注入来源；各模块保有私有 SQL、原单据、审核/取消事务与审计，中心不创建新流程表。全局时间＋来源＋ID 游标合并授权结果，源服务失效时返回明确的部分失败。入口能力 approval.self 仅做会话保护，不参与可分配权限；管理范围继续取原来源。见[审批中心指南](integrations/approval.zh-CN.md)。

## 统一发放边界（2026-09-21，已发布）

宿主向 welfare 注入 exchange.MatchRewardDelivery（纯快照匹配）与 WelfareGrantTx（同事务币账），welfare 不导入 exchange 私有 store。成长/补损/旗舰共用福利 River 自动核验与取消事务边界；兑换继续使用自己的订单状态机并共用精确物品算法、EVE 全局交付合同占用。不是让前端或游戏职务自动授予发放权限，角色归属和当前权限在事务内复查。


Goose 42（已随 `v0.1.0-welfare-cancellation-20260921` 发布）增加福利补损 cancel_requested 状态：取消审核与 River 合同结算共用福利事务锁/版本校验，已完成付款优先；仅取消确认释放 KM 资格，交付合同占用永久保留。规则见 [福利指南](integrations/welfare.zh-CN.md)。


Goose 41 的 gn_settlement_reference 仅为数据库编号工具，不读取跨模块业务表；exchange 通过列默认值、welfare 通过插入触发器生成，编号存于各自私有表并唯一约束。旧值保留；welfare 的读取/列表/自动匹配共同使用 reference，不再在页面或后台拼接新编号。

补损现金自动核验复用宿主注入的 EVE RedemptionContracts/ClaimDeliveryTx 和 identity 绑定/账号锁；welfare 私有 detail 保存审批策略、payment_status 与自动合同证据，沿用共享 River welfare 任务和持久时钟，不跨模块查询 store。完整编号查找、金额与授权核验在本地事务发布前后分层执行，详见[福利指南](integrations/welfare.zh-CN.md)。

旗舰补贴复用宿主注入的 EVE PurchaseContract / DeliveryContractTx / ClaimDeliveryTx。welfare 保存购舰证据并计算 ISK 补贴，交付通过既有 River 队列核对现金合同，不经过 exchange 币账；SQL 及账号合并冲突检查归福利私有 store。见[福利](integrations/welfare.zh-CN.md)。


成长福利通过宿主注入的配装先修/技能检查服务自动核验，申请发布事务复核配装与技能方案版本。同账号同船型共享终身一次资格，依据不可变申请船型、保留的领取历史/配置审计及唯一预留；原福利合并参与方同步检查跨项目船型冲突。无新增迁移，详见[福利接入](integrations/welfare.zh-CN.md)。

Goose 38/39：exchange 交付证据与时钟归自身私有 store，宿主注入 EVE RedemptionContracts / ClaimDeliveryTx 和 identity 活跃账号锁。共享 River exchange 队列负责本地缓存核对；EVE 全局交付合同占用同时被 welfare 使用并回填其已有占用。订单与证据通过外键保持账号合并归属，此为 Goose 38/39 当时的边界；当前福利统一自动核验规则见上方“统一发放边界”。见[兑换指南](integrations/exchange.zh-CN.md)。

奖励库核价经宿主注入 market.EstimateItems，exchange 从自身保存的奖励/配装快照汇总物品，使用原始吉他中间价、不叠加估价比例。报价前后重查管理员及版本，网络不占业务写事务；前端预览后通过原版本化配置端点保存，既有订单价格不变。见[兑换指南](integrations/exchange.zh-CN.md)。

统一实物奖励库由 exchange 私有 store 持有（Goose 37），兑换上架复用同一奖励 ID，内容版本独立于库存/报价版本；订单保存完整快照。welfare 通过宿主注入 LibraryReward 调用 exchange.WelfareReward，配置时检查管理员、目标军团和版本，不跨模块读取 SQL。果壳币仍归独立福利配置与币账，库不能包含币。详见[兑换与奖励库](integrations/exchange.zh-CN.md)。


成长福利通过宿主注入 GrowthFitting 读取授权军团配置，配置与申请保存奖励/配装快照，完成时经 exchange.WelfareGrantTx 在同一事务入币；跨模块不读取私有 SQL。动态项目复用现有福利规则、资格预留和账号合并，运行要求见[福利](integrations/welfare.zh-CN.md)。

请求语言仅作用于展示：宿主 locale 中间件读取 Accept-Language，StaticDataService 批量解析当前语言，后台保持默认中文；福利历史证据按响应投影，业务判断/持久化原证据/ESI 缓存独立。详见[界面语言](ui/language.md)。

## 界面语言

前端 `lib/i18n.ts` 与英文词表统一管理固定文案，中文为默认。切换偏好存于浏览器并重新加载当前 URL，涵盖模块级标签与懒加载页面；不改变模块协议、权限 ID、数据库内容或 ESI 请求。原始内容及 SDE 名称保持现有策略。开发与行为约定见[界面语言](ui/language.md)。

## PVP 核价协作

宿主注入 market.EstimateItems 与 EVE PurchaseContract，welfare 不导入其他模块 store。按服务端损失类型 ID 复用同一报价引擎、共享 ESI 缓存及 market 配置；网络先于业务写事务。核价证据保存在既有案件 detail 和审计，账号合并自然保留，不新增迁移。申请/重核价生成快照，批准验证版本及金额或明确的人工核价，游戏交付仍走独立确认流程。见[福利指南](integrations/welfare.zh-CN.md)。

## 市场估价边界

合同快速估价经宿主注入 EVE `ReadContract`，market 不访问 EVE 私有 SQL。合同列表复用受保护的现有 EVE API；估价端点从完整本地快照选单侧物品，报价前后复核对象权限、有效状态及内容摘要，复用统一 `EstimateItems`。无合同写入、迁移或新队列。

Goose 36：独立 market 模块持有全局估价比例与配置审计。宿主注入 EVE 精确 SDE 名称解析和公开吉他报价，仍由 EVE 共享客户端处理缓存/分页/限流；market 不导入 EVE 私有 store。估价输入不持久化，没有账号归属或币余额需要合并；历史配置审计保留原操作者。新模块按启用清单注册，market.self 不开放成员私有数据，比例保存另查当前管理员。见[市场指南](integrations/market.zh-CN.md)。

## 补损交付合同边界（历史首次引入，当前统一规则见上文）

Goose 35：welfare 持有确认快照、全局合同预留及持久检查时钟，宿主注入 EVE 的 DeliveryContracts / DeliveryContractTx。EVE 复用既有对象鉴权，读取本地合同及物品，提交事务锁合同/明细避免证据并发变化；网络抓取仍由 EVE 原同步器负责。共享 River 新增 welfare 队列及 delivery-scan/check.v1，管理员确认前不改案件，后台复核原确认人当前权限。账号合并沿用福利原参与方，不复制来源权限。详见[福利指南](integrations/welfare.zh-CN.md)。


## ISK 钱包边界

Goose 33/34：EVE 私有 store 负责钱包观察、原始记录区分与可恢复游标；独立 wallet 模块提供 read-only HTTP，由宿主注入 EVE 数据、SDE 与对象权限回调。复用共享 River/ESI，不直接导入 EVE store。wallet.self 只允许进入模块，读取仍须有效绑定/管理员或军团业务＋分部授权。数据按角色/军团归属，无新增账号级币账。详见 [钱包指南](integrations/wallet.zh-CN.md) / [English](integrations/wallet.en.md)。

2026-09-24 本地待发布：工作台个人钱包汇总由 wallet 模块在当前账号范围内筛选可读角色，经宿主注入的 EVE `WalletSummaries` 批量读取本地余额及当月流水；不跨模块访问私有 store，不包含军团分部或跨成员数据。明细端点与原有对象鉴权继续独立。


## 福利与考勤服务协作

宿主 `wireWelfareAttendance` 将 attendance 的 `ConfirmedReimbursementLosses`、`LockReimbursementLoss` 注入 welfare 的 `AttendanceLosses`、`GuardAttendanceLoss`。列表先鉴权原始角色损失，再按当前绑定账号批量读取同军团的已确认出勤损失；写入按原单账号检查。福利不导入考勤私有 store。军团补损提交/批准在绑定、凭据与账号锁之后、福利锁之前，对活动、出勤条目、已确认损失取共享行锁并重查，避免确认状态被并发撤销。考勤 UI 未启用不影响校验已有记录；无服务或无关联拒绝军团补损，PVP不受此门槛限制。

Goose 32：角色损失归 EVE 私有 store，使用共享 River、授权代次及发布 fence；福利经宿主注入本地读取和提交前证据守卫，保存申请时的独立快照。不跨模块读取 EVE SQL，不因原始证据解绑删除而删除已受理福利历史。详见[损失同步](integrations/character-losses.zh-CN.md)，已发布。

Goose 31 引入 welfare 模块，后续已发布，交付四项福利受理及管理员主动发币。SQL 归模块私有 store，规则/资格/申请/交付/审计与 exchange 币权益通过宿主事务服务协作；唯一币账仍在 exchange。identity 提供活跃账号锁契约，welfare 始终参与账号合并；有冲突的预留拒绝合并，原归属和历史审计保留。当前自动核验及人工审核边界见[福利指南](integrations/welfare.zh-CN.md)。

Goose 30：已实现会话绑定的 EVE SSO 账号合并证明、预览与确认。identity 通过宿主注入各业务的 `MergeAccountTx` 同事务迁移角色、PAP/币/订单及配装记录，保留 `original_account_id` 和原审计。目标主角色/社区/RBAC 保持原样，来源账号停用；各模块通过数据库活跃账号契约阻止旧账号迟到写入。详情见[账号合并](integrations/account-merge.zh-CN.md)，下方“合并待实现”为历史阶段记录。

Goose 26：exchange 来源配置支持手动/自动兑换，默认手动。attendance 经宿主注入 CoinConversion 事务接口获取报价/提交；服务端从活动已发PAP生成权益，不接收浏览器金额。手动入口逐次检查站点管理员及军团对象权限；自动回调只兑换新增积分，两种模式减分均冲正超额已兑换币。活动、钱包及审计采用同事务锁和幂等，模块私有store边界不变。

2026-09-15，Goose 25：独立 exchange 模块拥有果壳币钱包、来源权益及流水、奖励和订单。attendance 经宿主注入事务回调在发分、更正、撤销时同步币权益；仅注册已实现的 PAP 来源，不跨模块导入 store。币精度 0.01，比例按权益快照固定，支持欠额。配置/发放逐次检查站点管理员。详见[统一兑换](integrations/exchange.zh-CN.md)。

2026-09-15 考勤扩展：Goose 22 的 attendance 私有表保存每次舰船快照、有限装配证据、损失候选及持久任务。宿主注入 EVE BattleGateway 和 StaticDataService；授权/ESI 请求归 EVE，考勤不导入 eve store。platform/jobs.Extension 在共享 River 创建前注册模块处理器与周期任务，独立 attendance 队列并发 2；不是动态插件执行。详细生命周期见[考勤指南](integrations/attendance.zh-CN.md)。

更新：2026-09-14。本文区分当前实现和后续设计，是新增业务模块的结构基准。

采用 **Go 模块化单体 + 显式模块注册 + React 页面模块**。借鉴 SeAT 的包、服务提供者和统一扩展入口，将认证、社区确认、游戏同步等业务放在独立边界内。新模块随项目构建发布；当前没有运行时插件安装、热卸载或沙箱。

## SeAT 参考与项目适配

SeAT 的主仓库通过 Composer 组合核心包；插件采用 Laravel 包，通过 Service Provider 注册能力。参考 [主仓库依赖](https://github.com/eveseat/seat/blob/master/composer.json)、[插件基类](https://github.com/eveseat/services/blob/master/src/AbstractSeatPlugin.php) 和 [WebServiceProvider](https://github.com/eveseat/web/blob/master/src/WebServiceProvider.php)。

| SeAT 机制 | 本项目采用的方式 | 当前状态 |
| --- | --- | --- |
| Composer 包 + Laravel Service Provider | Go 包 + `module.Definition` + 宿主显式组装 | 已实现 |
| 包元数据与依赖管理 | ID、发布版本、宿主 API 版本、依赖检查与拓扑排序 | 已实现；没有版本范围解析器 |
| 注册路由、菜单、权限 | 后端按模块限定 API 路径；前端统一声明；宿主会话鉴权和 access 军团策略 | 已实现；管理页面已实现；Squads 待实现 |
| 插件查询、模型、迁移 | 每模块私有 store；Goose 统一编号迁移作为发布清单 | 已应用于全部已交付业务模块 |
| 事件、队列与集成插件 | 业务服务接口 + River 持久化任务 + 后续外部平台适配器 | ESI 队列已实现，外部适配器/通用事件总线未实现 |
| 安装包并完成升级步骤 | 代码审查 → 构建 → 显式迁移 → 发布和重启 | 构建、迁移、备份、systemd 与 OpenResty 发布已实现 |

SeAT 包的安装与更新参见 [官方包开发说明](https://eveseat.github.io/docs/developer_guides/package_development/) 和 [社区包说明](https://eveseat.github.io/docs/community_packages/)。这里的模块私有数据访问和窄服务接口是本项目的设计加强，不能理解为 SeAT 强制隔离机制。

## 代码结构与调用方向

前端页面注册同时提供共享 `preload()` 与 lazy 组件：当前 URL 对应的本地代码块可与模块目录请求并行下载，可见导航在指针进入／键盘聚焦时预载代码。预载只下载本站构建产物，不渲染页面、不激活被停用模块、不访问私有 API；服务端目录与会话门禁仍决定页面呈现。合同模块内部统一查询配置，已登录且 URL 明确指定对象时并行读取范围与对象，范围确认前不展示结果；每个业务请求仍由服务端独立鉴权。

```text
cmd/server
  └─ internal/app                  宿主组装：连接池、服务实例、注册表、HTTP
      ├─ internal/module           纯模块契约与校验，不连接数据库
      ├─ internal/httpapi          HTTP 中间件、统一响应、公共运维接口
      └─ internal/modules/         system、identity、eve、access、community
          └─ <module>/internal/store  各模块自己的 SQL 和 sqlc 生成代码

web/src/App.tsx                    公共布局、焦点管理、加载和失败状态
  ├─ app/modules.ts                构建时明确引入可信前端模块
  ├─ app/module-registry.ts         统一页面声明与版本检查
  └─ modules/                      模块页面、请求契约、查询缓存键

migrations                        经审查的统一编号 Goose 发布迁移
```

跨模块调用经宿主注入的业务服务接口完成。接口由使用方按需要定义，暴露业务操作和 DTO；不把全局服务定位器、原始令牌或其他模块的 SQL 查询对象传给调用方。模块不能导入宿主 `internal/app`，避免依赖倒置。

Go 的嵌套 `internal` 目录限制其他模块直接导入 `system/internal/store`。这是编译期访问限制，同进程仍共享内存、连接池与数据库凭据，不提供恶意代码隔离。确实需要运行不受信任的第三方代码时，另行设计独立进程、权限受控的 API 和资源隔离。

## 注册、启用与权限

1. 宿主列出已编译的模块。未设置 `MODULES` 时按 `internal/config/config.go` 默认启用 system、identity、eve、access、community、attendance、sentry；`.env.example` 的显式示例包含更多模块；system 是必需模块。access 依赖 identity/eve，sentry 依赖 identity/access。显式 `MODULES=system` 可以仅运行底座。未知、重复或依赖不完整的配置导致启动失败。
2. 注册表检查唯一 ID、`x.y.z` 发布版本、宿主 API 版本和依赖环。当前宿主 API 为 1，依赖要求精确 API 版本；发布版本用于识别代码，不代表依赖兼容性推断。未来单独升级业务服务契约时，应新增明确版本字段，不能只修改显示版本。
3. 按依赖先后形成模块目录及路由列表。Definition 不包含通用启动/停止回调；EVE River worker 由 Application.Run 与 server 显式管理。拓扑排序不等于通用后台插件生命周期。
4. 模块声明相对路径，宿主加上 `/api/v1/<模块 ID>`。禁止任意路径、通配路由及重复路由；变量名不同但形状相同的路径也视为重复。
5. 接口默认受保护，必须声明模块命名空间下的权限并由宿主提供 Authorizer。identity 验证会话/CSRF，再委托 access 检查本站权限；未知能力拒绝。对象端点还必须解析服务端目标并调用军团策略，不能只检查 access.self。未启用鉴权模块时受保护接口使启动失败。只有显式 Public 端点可匿名访问。
6. `GET /api/v1/modules` 只暴露启用模块的 ID、版本和 API 版本。前端从本地已构建模块中选择页面；未知的纯后端模块不加载 UI；必需模块缺失或 API 不兼容则显示可重试错误。

前端页面与导航来源一致，并按页延迟加载。登录页 `/login` 声明 `navigation: false` 和 `layout: "standalone"`，宿主不展示工作台侧栏与顶栏；登录后进入保留工作台布局的 `/account`，未登录访问账号页会返回登录页。页面路由仍由注册表管理。模块目录不下发脚本地址，不动态执行远程代码；隐藏页面不等于鉴权，独立布局也不代表全站登录限制。资料补全和权限过滤由宿主统一处理，每个后端业务接口仍需对象范围检查。

启用配置在进程启动时生效，变更后需要重启。停用可选模块会移除其 API 和页面入口，不删除历史数据。当前模块清单见[技术栈](tech-stack.md)；没有运行时管理模块开关的界面。

## 数据归属、迁移与任务

- 模块拥有自己的 SQL 查询；生成代码只用于本模块。新增模块在 `sqlc.yaml` 增加独立配置块和输出目录，不重新建立所有业务共享的 `internal/db`。
- 数据库迁移暂采用根目录 Goose 全局编号，作为一份受审查的发布清单；迁移头部标明所属模块。API 启动不执行迁移，停用模块不执行 down。未来模块增多再引入迁移清单生成工具，避免过早维护多个冲突的版本表。
- 当前 `platform_metadata.schema_version=1` 只表达 foundation 支持的结构；它不是每安装一个模块就自增的全局插件计数器。未来模块检查自身所需结构，Goose 记录发布迁移进度。
- 跨模块即时读取走服务接口；提交资料、确认入群等状态写入由数据所属模块执行。报表通过明确的只读查询契约或汇总数据访问业务信息，不任意跨模块修改表。
- 当前 ESI 使用 River，业务写入和可靠事件/任务入队在同一个事务完成；确认通知采用至少一次投递和幂等消费。队列中只携带必要业务 ID、版本和事件 ID，不传播 EVE refresh token。
- 任务名和事件类型带模块命名空间与契约版本，例如 `community.profile-updated.v1`。新旧消费者兼容、重试、失败排查、停用时积压任务处理，以及先停止接单再退出 worker 的流程，必须与第一个后台模块一同实现；已有 ESI River 队列，没有进程内通用事件总线。

## 认证与社区接入边界

identity、eve、access 和 community 已注册，外部适配器与报表仍为边界规划。当前实现与真实联调状态见 [登录指南](integrations/eve-login.zh-CN.md)。

| 边界 | 负责 | 不向其他模块暴露 |
| --- | --- | --- |
| identity | 本站账号、会话、角色绑定、统一操作权限和对象范围上下文 | 会话凭据、直接修改账号表的能力 |
| eve | EVE SSO、ESI 授权令牌、刷新协调与 ESI 请求 | 原始 refresh token |
| access | 本站 RBAC、SeAT 风格军团权限、授予与审计 | 直接修改授权表的数据库入口 |
| community | QQ 号和 KOOK 昵称、资料完整度、确认状态、QQ 群入群申请及群范围身份绑定 | 直接写确认字段的数据库入口 |
| qq / kook 适配器 | 把官方 QQ/KOOK 外部事件转换为 community 服务操作 | 绕过业务确认规则的权限 |
| reports | 有权限范围的汇总、明细与导出 | 无限制读取全部成员信息的能力 |

认证与社区资料按用户已经确认的要求实施：

- 使用 EVE SSO 登录及 ESI 授权。首次登录后填写 QQ 号与 KOOK 昵称；两项填完才完成本站资料流程。QQ/KOOK 暂不做 OAuth 登录，也不因填写成功就标为已验证。
- 资料是否齐全与是否完成 QQ 入群、KOOK 进入确认分别保存，后者不能混作登录条件。QQ 入群审批由官方 Bot 回调/分页同步驱动；KOOK 昵称不视为可靠的唯一平台身份。
- 修改 QQ 号或 KOOK 昵称时撤销对应旧确认；确认事件应关联资料版本，避免迟到的机器人事件确认新资料。确认记录保留来源、时间和操作者/事件 ID；当前状态为 unfilled/pending/confirmed，当前成员接口不能创建确认记录。
- QQ 官方机器人通过 community 的官方 HTTP 回调提交事件，按 Bot Secret 派生的 Ed25519 签名、时间窗和官方事件 ID 做验签/幂等，再由 community 校验配置群、一次性申请码、当前 QQ 资料版本，并在 `GROUP_MEMBER_ADD` 后写入群范围绑定与确认状态。回调未送达时，管理员可调用官方入群申请分页接口执行幂等补偿同步。机器人不持有本站数据库或 EVE 令牌；请求中的 QQ 身份只能取自官方事件的 bot-scoped openid，不能由成员浏览器直接调用。
- 登录闭环通过 identity、eve、community 的业务接口组合；接入机器人不需要改写 EVE 登录流程。

旧认证 SQL 草稿已清理，当前结构只以正式 Goose 迁移及模块私有查询为准。

## 一人多游戏账号与多角色绑定

用户补充一个人可能持有多个 EVE 游戏账号。本站按“一个成员账号绑定多个 EVE 角色”建模，角色可以来自不同游戏账号。绑定、主角色管理、独立重新授权和解绑已实现；显式账号合并已通过 Goose 30 实现；游戏角色买卖或身份转移恢复不属于自动合并。

已按固定提交核对 SeAT 的 User、SSO 和个人资料控制器，采用“一个用户、一个主角色、多个小号”的模型。操作规则、源码依据、适配差异及验收条件见[多角色设计（中文）](integrations/seat-multi-character.zh-CN.md) / [English](integrations/seat-multi-character.en.md)。主角色、SSO 登录角色和当前查看角色分别保存语义；切换查看不更改认证身份。首次登录角色默认为主角色，任一有效绑定角色可登录同一用户。

```text
本站成员账号
  ├─ QQ / KOOK 资料与社区确认（账号级）
  ├─ 本站角色与手工授权（账号级）
  ├─ EVE 角色 A → 独立授权、军团归属、职务、同步状态
  ├─ EVE 角色 B → 独立授权、军团归属、职务、同步状态
  └─ EVE 角色 C → 独立授权、军团归属、职务、同步状态
```

- `identity_characters.user_id` 支持一对多，`character_id` 全局唯一；`identity_users.main_character_id` 由同用户复合外键约束。`identity.Service.Complete` 处理明确的 login/link/reauthorize 意图，`SignIn` 保留普通登录语义。
- 登录、绑定角色和更新角色授权须区分流程意图。只有已登录成员主动“添加角色”才将新角色绑定到当前账号；普通登录不能因为浏览器已有会话就隐式关联两人身份。
- 添加角色需要为该角色重新完成 EVE SSO 验证。一次性 flow 由服务端记录绑定意图、目标本站用户、原会话和请求范围；回调重新检查会话与目标，不能接受浏览器自行指定归属。
- 主角色用于展示和默认选择，不是权限来源；当前会话角色、展示主角色和账号已绑定角色列表分别处理。任一绑定且身份有效的角色完成登录后，应回到同一个本站账号。
- QQ 号、KOOK 昵称及社区确认跟随本站成员账号，添加小号不重复填写；不能根据同名、相同 QQ、KOOK 昵称或同军团自动识别并合并为同一个人。
- 每个角色单独保存授权范围、令牌和同步状态。一个角色撤销授权，不删除其他角色的凭据；该角色失效的职务事实必须退出权限判断，不能用另一个角色的令牌代替访问。
- 本站手工授权按成员账号保存；游戏职务仍与具体角色、军团和有效快照绑定。聚合多角色权限时保留原目标范围，不能把 A 军团的 Director 身份用于 B 军团。
- 同一角色不能同时属于两个本站账号。已归属其他账号时不直接抢绑；此前分别登录形成的重复本站账号，需要独立的归属验证和合并流程，权限与社区确认不得未经审查直接相加。
- 解绑、角色转移与主角色变更要处理关联会话、凭据、快照和审计。解绑主角色前先选另一个有效绑定角色；常规解绑不允许移除最后一个有效登录角色。账号合并使用独立 SSO 验证及预览确认，不搬移来源社区确认或手工授权；角色转移恢复另行设计。
- 角色绑定与 ESI token 生命周期分离；撤权保留角色关系并排除失效职务事实。新绑定及重新授权沿用本站当前 scope profile，不复制主角色可能缺项的旧 scopes。

迁移 `00007` 增加主角色、flow 意图及绑定审计。宿主注入 EVE 凭据写入/清理回调，identity 在同一事务中完成归属和会话变化，业务 SQL 仍归各模块私有 store；SSO 网络交换在事务外进行。已覆盖多角色登录、冲突、回滚、并发、解绑与跨军团权限测试；真实操作证据与未验收边界见[项目状态](project-status.md)及[待办](backlog.md)。

## 当前交付范围

ESI 接入已分为 `ESIService`、凭据服务、`internal/esiclient` 与资源 worker。宿主可注入无令牌参数的 Request 服务，HTTP/共享缓存/限流与业务范围分离，所有请求复用同一实现。Goose 18 在 eve 私有 store 保存令牌观测，管理查询只读元数据，不解密凭据；详见[接入层与令牌观测](integrations/esi-client.zh-CN.md)。

已实现模块契约、依赖与路由校验、权限包装入口、system 模块、模块私有查询、前后端启用目录、页面拆分及失败处理。新增模块步骤见 [开发约定](module-development.md)。

EVE 登录、职务同步、SeAT 风格权限和本站 RBAC 已实现，现有应用凭据已配置；登录与私有 ESI 已投入使用，未验收的游戏写入/交付路径见[待办](backlog.md)。QQ/KOOK 手填资料、完整度门禁、按字段失效旧确认和 QQ 官方群入群审批已在本地实现，见[社区资料说明](integrations/community-profile.zh-CN.md)。KOOK 机器人、Squads、通用可靠事件与后台插件生命周期尚未实现。

角色资料与职务同步由 River 调度，业务目标持久化到期时间，使用授权代次、租约和 fence 防止旧任务发布。角色抓取预算 55 秒、令牌刷新锁内预算 10 秒，网络抓取位于业务发布事务之外。缓存与限流通过 PostgreSQL 跨实例共享。宿主先停止 worker 再关闭连接池，详见 [ESI 同步运行指南](integrations/esi-sync.zh-CN.md)。资源预算在部署与压测时单独确定。

## 合同资源（2026-09-14）

Goose 17 在 eve 私有表增加由合同参与方计算的 in_scope，区分 ESI 返回来源与军团自身业务。军团自身发布、指定接收或实际接受才进入页面和明细同步，个人范围保持原规则。列表、对象读取及明细任务共用数据库范围判定；原军团对象鉴权仍在宿主执行。ESI 页码照常推进，基础元数据保留，范围外明细被 fence 并停止请求。契约细节见中英文合同指南。

合同保存在 eve 私有 store，个人与军团通过 owner_kind/owner_id 隔离。列表每页一次发布事务，断点和后续物品/出价任务同事务提交；明细有独立租约/fence 和授权代次检查。同一军团选择有效授权来源，发布前重新检查，其他来源不重复抓取。新增 `eve_contracts` 并发 1，角色队列仍并发 2。状态接口仅暴露元数据；合同内容通过独立 eve.contracts.read 路由和每请求对象检查提供。宿主经 identity.ActiveCharacters、UserForCharacter（有效绑定）与 access.IsAdministrator／Account.Can 注入可查看范围，个人数据允许本人或站点管理员读取，eve 不直接访问其他模块 store。/contracts 查看个人与军团合同；军团阅读能力为 corporation.contract，数据库新增 Goose 12 名称缓存及筛选索引。详见[合同指南](integrations/contracts.zh-CN.md)。

## SDE 名称资源

eve 私有 store 保存版本化 type-names 及活动指针，Go 运维命令通过 eve 服务调用流式导入。Goose 13 隔离公开名称缓存语言，14 新增 SDE 名称表。合同物品按活动 SDE 批量读取中文／英文，缺失类型仅回退已有 ESI 名称缓存，不在渲染时逐项访问网络。COPY、版本和活动指针在同一事务发布，导入锁保证串行，失败保留当前数据；已接入独立 River SDE 队列和持久化更新时钟，尚无管理 UI。`StaticDataService.TypeNames` 为合同及后续业务统一解析名称；宿主启用公共 SDE 更新不依赖 SSO 凭据，角色同步就绪状态仍单独判断。Goose 15 提供更新租约/fence、失败退避和手动固定版本，回退后自动任务不能覆盖固定版本。其他模块需经服务接口复用，禁止导入 eve store。范围及命令见[SDE 运行指南](integrations/sde-names.zh-CN.md)。

## 管理员成员读取

`/members` 属于 access 模块，搜索复用 identity.SearchMembers 的 25 条分页；详情经宿主组合 identity.Characters、access.Account 与可选 community.Get，模块只接收展示 DTO。access.members.read 为管理员标志专用、不可分配的已知读能力。合同通过 member 参数选择成员，对象读取经独立 LookupOwner 检查，避免枚举全体成员鉴权。同步 GET 使用 CanRead，POST 保持 Owns。管理员撤销后后续请求即失权；不存在／blocked／解绑角色仍拒绝。

## 合同列表导出

合同 CSV 导出属于 eve 前端，只消费既有列表 DTO，不引入 reports 模块、跨模块 store 查询或后台任务。按选定 owner 与业务筛选顺序读取所有分页，每页经现有对象鉴权；最多 10,000 条，失败整体终止、作用域变化取消，数据仅在当前浏览器生成文件。没有新增权限能力，能读取的数据可导出；不扩展为全成员聚合导出，也不声称跨页为事务快照。字段与文件规则见[合同指南](integrations/contracts.zh-CN.md)。

Goose 19 另存 ESI 令牌桶和接口消耗聚合：HTTP 客户端记录真实响应头、缓存和本地等待，管理 API 只读独立元数据。按分组及角色/公共出口隔离，累计值与本地限流预算分开；观测失败不影响业务发布。

Goose 20 在 eve 私有 store 保存逐笔预算账目，由同一 esiclient 完成短事务预留/结算，HTTP 位于事务外；缓存、配额和业务发布使用独立时钟。嵌入的官方兼容日期目录提供首次分组配置，实时响应校正；聚合观测与本地预算分别维护，详见[客户端指南](integrations/esi-client.zh-CN.md)。

## 军团考勤与在线观测

`attendance` 为已实现模块，依赖 identity/eve/access；事件、条目、审计归其私有 store，在线目标与样本归 eve。宿主注入绑定批量读取、军团范围权限和 EVE 无令牌业务网关。点名在网络后以 identity → credential → event 锁序提交；在线报表可在同一数据库事务中查询，避免占用一个连接再等待另一个连接。事件保留原账号归属；在线解绑清理并按采样军团隔离。运行与回退见[考勤指南](integrations/attendance.zh-CN.md)。

## 集结分账目（Goose 24）

attendance 拥有角色-活动余额及不可覆盖的调整流水；按原本站账号归属累加多个角色。事件行锁串行化发分与名单生命周期，已发分活动先撤销再重开，所有分值更正仅记差额。无独立网络采集、无新队列或通用积分插件。细节见[考勤指南](integrations/attendance.zh-CN.md)。

## 舰船配置（Goose 27）

fittings 私有 store 维护军团方案库、审计与游戏保存记录（Goose 29），保留旧草稿；eve 维护角色快照、River 与写入网关。宿主注入有效绑定、管理员、军团范围、StaticDataService 及游戏保存业务接口，跨模块不导入 store。当前页面不提供模拟；固定 SDE 参考只用于 EFT 与前置技能解析，skills 独立保存生成的要求方案。详见[中文指南](integrations/fittings.zh-CN.md)。

## 技能模块（2026-09-15）

`skills` 依赖 identity/eve/access，经宿主注入角色、军团策略和快照；私有 store 维护要求方案及审计。EVE 拥有 skills/skillqueue 原始快照，沿用 Goose 27 表名并在 Goose 28 扩展资源白名单。fittings 和 skills 共享技能同步目标，独立 skills 模块控制队列目标。本站方案写入不调用 ESI，成员技能读取仍校验有效绑定和当前管理员身份；军团能力只开放方案及对应要求检查。接口及边界见[技能指南](integrations/skills.zh-CN.md)。

## 公开介绍与成员入口（2026-09-23，已发布）

system 模块声明 `system.landing` 声明用于 `/`，App 对这一固定页面单独懒加载，避免介绍页挂载私有目录、管理权限或业务查询。其余路径仍使用现有模块目录和访问控制；工作台迁移至 `/workspace`。登录表单和 SSO 回调不变。eve 模块显式开放固定军团的公开资料投影 /api/v1/eve/public/corporation，宿主注入共享 ESIService；仅访问官方匿名路由，不读取成员令牌或私有数据。其他业务端点保持鉴权。见 [首页边界](ui/homepage.md)和[公开资料接入](integrations/public-corporation.zh-CN.md)。
