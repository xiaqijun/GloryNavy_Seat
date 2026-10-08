# SeAT 风格的军团权限

> 状态索引（2026-09-23）：本指南对应的业务已上线；未交付的扩展与真实验收缺口见[待办](../backlog.md)，当前版本见[项目状态](../project-status.md)。下文带日期/Goose 的发布前记录保留当时语境，不能据此认定当前未发布；升级执行仓库最新迁移，不停在模块初始版本。

2026-09-16 更新：本地钱包已交付，ManageableCatalog 新增 corporation.journal、corporation.transaction 和 wallet 1…7 分部。读取同时检查业务能力与分部；ESI 来源角色独立校验 Accountant/Junior_Accountant/Director/CEO，分部名仅 Director/CEO。管理员可读取有效绑定成员钱包，详见 [钱包权限](wallet.zh-CN.md)。

2026-10-07 贷款模块已随 `v0.1.0-loan-guarantee-array-20261007-r2` 发布：`corporation.loan` 可由管理员配置给军团贷款池管理者，用于军团出借管理、申请审核及担保/抵押决定；个人出借池仍由出借账号本人管理。所有贷款写操作继续执行对象、角色和本站权限检查，不能由目录条目替代鉴权。真实 Tranquility 合同和成员/管理员现场验收仍待完成。详见[贷款指南](loan.zh-CN.md)。

## 专员角色与合同处理角色（2026-10-08）

“贷款专员”“补损专员”是本站权限角色，应在权限管理中把对应军团范围授予本站账号。权限角色决定业务审核、处理和读取范围，不由 EVE 游戏职务自动获得，也不因绑定某个游戏角色而自动获得。

合同接收方是另一项配置：需要选择一个仍绑定本站账号的个人 EVE 角色。系统按该角色所属本站账号重新检查本站权限、军团对象和业务单据；角色失效、解绑或所属账号失去权限时，合同处理和核验应拒绝。贷款专员的合同处理角色已在贷款设置中上线。补损专员使用独立的 `corporation.welfare.compensation`，只允许审批和处理 `srp`、`solo` 补损；成长、旗舰、果壳币和规则配置仍需要完整的 `corporation.welfare` 或站点管理员。该权限不替代账号原有普通成员能力。


Goose 32 原始损失读取限本人有效绑定角色或当前站点管理员，需核对当前绑定及军团。`corporation.welfare` 只允许依福利单据范围审核已提交证据，不开放其他成员全部原始损失。新原始记录不通过 QQ/KOOK 或同名关联；scope 移除、失效或游戏所有者变化拒绝旧证据。见[损失同步](character-losses.zh-CN.md)。

Goose 31 本地新增 `welfare.self` 入口与可配置 `corporation.welfare`（福利审核/交付）。后者必须明确配置范围授权或具备当前站点管理员身份，不从 CEO/Director 等游戏职务自动获得。规则、历史资格登记、主动发币及冲正仅站点管理员；成员只能操作自己的申请，管理员也不能批准或交付自己的申请。对象读取仍核对当前有效绑定和军团范围；福利授权不扩展合同或 ESI 数据权限。见[福利指南](welfare.zh-CN.md)，未上线生产。

更新：2026-09-14。对应 [English](seat-authorization.en.md)。

后续更新：登录授权已扩大为 SeAT 默认清单的 57 项当前兼容 scopes，并拆分独立 `/login` 与 `/account`。应用权限配置请使用 [最新清单和步骤](seat-login-scopes.zh-CN.md)，不能再仅勾选下文第一轮的职务 scope。本文件的军团权限策略和任务边界继续适用。

## 权限管理页面（2026-09-14）

入口 `/access`，包括角色配置、成员授权、操作记录。导航使用 `/access/me` 的 `can_manage`，每个管理 API 仍独立核验 `access.manage`；管理员或含该能力的角色可管理全部平台角色和账号授权。EVE CEO/Director 不自动成为本站管理员。QQ/KOOK 资料不完整时先在“我的角色”补全，社区确认状态不影响此门禁。

- 角色按 UUID 保存，`GET /roles` 返回十进制字符串 `version`。创建 PUT 必须带 `version: "0"`，更新带读取时的版本；DELETE 带 `?version=当前版本`。旧版本或已删除对象返回 409，失败不写审计。此为管理写接口的契约更新，旧调用方必须同步升级。
- 管理目录返回已交付能力，包括权限/同步管理、合同、考勤、技能、福利、钱包流水/交易和钱包分部，以及本地未发布的 `corporation.loan`；未实现的军团业务及资产分部仍不展示；底层 `Catalog` 的 SeAT 映射与历史授权兼容保持独立。页面编辑原样保留目录之外的历史授权，不默默清除。全局 `access.manage` 不接受对象过滤。
- `GET /api/v1/access/members?q=...&after=...`：按任一绑定角色名（大小写不敏感的字面子串）、精确角色 ID 或账号 UUID 搜索；每页 25 个账号，按账号 UUID 排序。返回主角色显示信息、角色数量、管理员标志及平台角色。响应 `items/next`，搜索改变时清空游标，不返回 QQ/KOOK 或认证凭据。
- `GET /api/v1/access/audit?before=...`：每页 50 条，按事件 ID 倒序，返回 `items/next`。审计包含操作者、动作、对象 ID、时间；不宣称保存了变更前后差异或历史显示名。授予/撤销继续使用已有 PUT/DELETE 用户角色接口。
- 管理列表由 identity 的成员目录服务和 access 的角色查询组合，私有 store 不跨模块导入。主角色仅用于展示；授予跟随本站账号，权限范围仍按具体军团判断。

升级先执行 `npm run db:migrate` 到迁移 `00009_access_management.sql`，然后重启 API、发布前端；foundation marker 仍为 1。首次管理员仍由运维显式指定已登录的角色（下方命令），不自动提升首个用户。本机权限设置记录留在数据库审计，不硬编码人员姓名。

当前已有角色管理 UI 和审计查询；Squads、角色继承、权限解释器、细粒度管理委托和完整审计差异仍待设计。配置 `access.manage` 等于授予全站角色管理能力，并非仅管理某个军团。

## 已实现范围

本轮实现 ESI 本人职务同步、SeAT 风格军团策略、独立本站 RBAC、管理 API 和角色页授权状态。Squads 自动分组、全军团成员职务目录、资产和财务数据采集未在本轮实现。目录中的资产等权限名是未来模块的稳定契约，不代表已经可以读取这些数据。

以 [SeAT CorporationPolicy](https://github.com/eveseat/web/blob/master/src/Acl/Policies/CorporationPolicy.php)、[EsiRolesMap](https://github.com/eveseat/web/blob/master/src/Acl/EsiRolesMap.php)、[角色同步任务](https://github.com/eveseat/eveapi/blob/master/src/Jobs/Character/Roles.php) 和 [官方授权说明](https://eveseat.github.io/docs/admin_guides/authorizations/) 为机制参考，使用 Go 重新实现。

## 权限判断

1. 先核验本站会话。未知权限始终拒绝，即使请求者是管理员。
2. 本站管理员可以访问已登记的能力；管理员身份由本机运维命令设置，不由首次登录或 EVE 职务自动产生。
3. 访问指定军团时，当前有效绑定角色中的该军团 CEO 或 Director 具有该军团权限。不会因此得到本站权限管理能力。
4. 其他游戏职务按照下面的映射授予该军团的对应能力。
5. 再检查用户的本站角色和角色内的权限过滤器。
6. 没有任何匹配即拒绝。

| 游戏职务 | 本站军团权限 |
| --- | --- |
| Accountant | 概览、钱包流水、交易明细 |
| Account_Take_1…7 | 对应钱包分部 |
| Container_Take_1…7 | 对应资产分部 |
| Auditor / Junior_Accountant | 概览 |
| Contract_Manager | 概览、合同 |
| Diplomat | 概览、成员追踪 |
| Security_Officer | 概览、安全审查 |
| Trader | 概览、市场 |
| Project_Manager | 军团项目 |

只用 `roles` 参与默认授权。`roles_at_hq`、`roles_at_base`、`roles_at_other` 独立保存并展示，不升级为全局职务；grantable roles 不是实际任职。CEO 来自军团资料 `ceo_id`，不是职务枚举。无职务成员不会自动获得军团概览权限，可以用本站角色配置。

本站角色的每项授权分别有 `corporations`、`alliances` 过滤器；两者 OR 匹配，空数组或省略两项表示不限制对象范围，这与 SeAT 4+ 一致。`access.manage` 是全站能力，禁止给它添加军团过滤器。游戏 ID 均使用十进制字符串传输，不接受客户端指定“当前目标的联盟”作为授权依据。

实现修正了所参考 SeAT 映射中的三个命名/索引问题：二号容器对应二号资产分部、合同能力使用 `corporation.contract`、项目经理枚举使用 `Project_Manager`。Director 根据本人角色接口确认；本轮不额外申请全军团成员职务接口权限。

## 本机配置与真实授权

已有 Client ID 和 Secret 保留在本机 `.env`。在 EVE 应用管理中为**现有应用**增加这个 scope：

```text
esi-characters.read_corporation_roles.v1
```

EVE 应用允许申请的 scopes 与玩家实际同意的 scopes 都需要满足；旧的无 scope 登录不能自动变成新授权。官方机制见 [SSO 文档](https://developers.eveonline.com/docs/services/sso/)。回调仍是：

```text
http://127.0.0.1:5173/api/v1/eve/callback
```

配置：

```dotenv
MODULES=system,identity,eve,access,community
PUBLIC_ORIGIN=http://127.0.0.1:5173
EVE_CLIENT_ID=本机既有值
EVE_CLIENT_SECRET=本机既有值
EVE_TOKEN_KEY=
```

```sh
npm run auth:key
npm run dev:external
```

`auth:key` 只在密钥为空时生成 32 字节随机密钥并写入 `.env`，不会输出或覆盖既有密钥。开启 access 且配置 EVE 登录时，缺少合法密钥会使启动失败。备份数据库时同时安全备份密钥；本轮没有在线密钥轮换工具，直接更换密钥会导致旧令牌无法解密，需要重新授权。

打开 `/login`。已有会话点击“授权军团职务”；未登录则正常使用 EVE 登录按钮。首次保存授权同事务加入 River 队列；队列积压和上游状态会增加等待时间。角色资源刷新按钮提交同步请求，不绕过 ESI 缓存。

## 同步、存储与失败处理

当前已迁移到 River，完整机制见 [ESI 同步运行指南](esi-sync.zh-CN.md)。令牌按角色加密保存，刷新先提交，再执行 ESI 抓取；403 仅阻断资源，不清空凭据。共享缓存与限流保存在 PostgreSQL，旧单事务轮询器已移除。游戏职务事实有有限有效期，换团时先失效旧事实；本站角色独立生效。正常退出只撤销本站会话，不撤销 EVE 授权。

## 管理 API

先让管理员本人完成一次登录，再由运维在本机指定其角色 ID：

```sh
npm run access:admin -- --character 你的角色ID
# 撤销：
npm run access:admin -- --character 你的角色ID --revoke
```

命令查找现有有效绑定账号并记录审计，不创建账号、不自动选首个用户。拥有该命令的运维具备数据库控制能力。管理 API 要求本站管理员或含 `access.manage` 的本站角色；写操作还要求同源 Origin 和会话中的 `X-CSRF-Token`。

| API | 作用 |
| --- | --- |
| GET `/api/v1/access/me` | 当前用户职务、本站角色和管理员标志 |
| GET `/api/v1/access/catalog` | 已交付功能的可配置权限目录，包含 `corporation.structure` 等已交付能力 |
| GET `/api/v1/access/roles` | 本站角色列表 |
| PUT `/api/v1/access/roles/{id}` | 创建/更新 UUID 标识的角色 |
| DELETE `/api/v1/access/roles/{id}` | 删除角色及其授予关系 |
| PUT / DELETE `/api/v1/access/users/{user}/roles/{id}` | 给 UUID 用户授予/撤销角色 |
| GET `/api/v1/access/corporations/{id}/summary` | 经过 `corporation.summary` 对象检查的军团资料快照 |

创建限定军团的财务角色，PUT 请求体示例：

```json
{
  "name": "军团财务",
  "version": "0",
  "grants": [
    { "permission": "corporation.journal", "corporations": ["你的军团ID"], "alliances": [] },
    { "permission": "corporation.transaction", "corporations": ["你的军团ID"], "alliances": [] }
  ]
}
```

角色 ID 由调用方生成 UUID。账号 ID 从当前用户的 `/api/v1/identity/session` 取得。修改角色、授予和管理员状态记录操作者、动作、对象与时间；已有审计查询页面，Squads 成员历史仍未实现。

最初授权底座新增 `00004_eve_authorization.sql`、`00005_access.sql`，当时 Goose 到 5（当前权限管理升级到 9），foundation marker 仍为 1。真实 EVE 角色授权和职务结果待应用 scope 更新后联调；本机数据库测试及浏览器测试使用隔离数据/模拟 ESI，不应标为真实 CCP 联调完成。

## 管理员查看成员数据（2026-09-14）

本站管理员可从 `/members` 搜索成员、切换其绑定角色，查看社区资料、平台角色、军团职务、ESI 同步状态及已同步的个人合同／物品／出价。管理员按当前账号标志逐次判断；普通 access.manage、eve.sync.manage 授权或游戏职务不获得此跨成员查看能力。资料完整度门禁仍校验操作者自身资料。

`GET /api/v1/access/members/{user}/data` 返回成员展示资料、绑定角色、权限事实及 QQ/KOOK 值和确认状态；社区模块未启用时 community 为 null。非法 UUID 返回 400，不存在成员 404，非管理员 403；响应禁止缓存。access.members.read 是已登记的管理员专用能力，不进入可分配角色目录，写入角色授权时拒绝；管理员标志不缓存，撤销后续请求立即拒绝。

个人合同和 GET 角色同步状态允许管理员读取他人有效绑定角色；不存在、解绑或身份 blocked 的目标仍返回 404。普通 /account、会话、绑号、解绑、主角色、重新授权、资料修改和本人 POST 刷新保持自身身份与操作边界。本次不扩大普通 CEO／Director 的个人数据访问规则，不新增 ESI scope、迁移或同步任务，不暴露凭据。页面说明见[成员资料](../ui/members.md)。

### 已交付考勤能力

`corporation.attendance` 已随活动/在线模块开放，可按军团与联盟授予；现有 CEO/Director 与本站管理员策略适用。名单抓取仍只使用操作者自己的有效角色且遵守游戏舰队访问权。管理员成员在线读取只检查当前站点管理员标志与有效绑定；不扩展成员令牌写操作。[完整范围](attendance.zh-CN.md)。

### 已交付建筑读取

`corporation.structure` 已开放只读建筑总览，按军团对象逐次检查当前角色/本站授权；读取 Upwell 建筑与 POS 燃料明细，不开放租用、Access List/Profile 或游戏内权限写入。[建筑指南](structures.zh-CN.md)。

## 军团技能要求

已开放 corporation.skills：维护目标军团本站技能要求，读取该军团有效绑定角色的要求检查结果。不会授予完整个人技能/队列读取；完整读取仅本人或当前站点管理员。前端隐藏不代替服务端逐对象判定。见[技能权限](skills.zh-CN.md)。
