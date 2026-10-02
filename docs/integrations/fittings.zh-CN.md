# 舰船配置：军团方案库与游戏保存

> 状态索引（2026-09-23）：本指南对应的业务已上线；未交付的扩展与真实验收缺口见[待办](../backlog.md)，当前版本见[项目状态](../project-status.md)。下文带日期/Goose 的发布前记录保留当时语境，不能据此认定当前未发布；升级执行仓库最新迁移，不停在模块初始版本。

2026-09-19 双语数据补齐：业务 API 携带 `Accept-Language: zh-CN|en`，服务器按请求上下文选择展示语言，并返回 `Content-Language`、`Vary: Accept-Language`；私有接口继续 `no-store`。类型/星系名称从本地 SDE 批量解析，缺失回退另一语言、已有类型名称缓存或 ID。合同空描述摘要和服务端固定错误按语言显示；角色名、玩家备注、合同描述保持原文，不改变业务 ID、权限、价格、合同内容令牌或 ESI 缓存。无新迁移、scope 或导入要求，仅本地。详见[界面语言](../ui/language.md)。

2026-09-15 改版，已发布生产 v0.1.0-fitting-library-20260915。[English](fittings.en.md)。本指南替代此前模拟工作区说明。

## 当前流程

- 当前站点管理员选择军团，粘贴游戏复制的 EFT 导入方案；可替换或删除，写入有版本检查和审计。军团库是本站数据，不宣称由 ESI 同步游戏军团方案。
- 成员浏览可见军团方案，选择自己有效绑定角色，点击“保存到个人配置”后确认“保存到游戏”。调用官方 `POST /characters/{character_id}/fittings/`，成功返回游戏 fitting_id。
- 个人方案继续读取 EVE 同步快照。游戏保存成功后，本站列表等待下一次受官方缓存约束的同步更新，不通过高频刷新绕过缓存。管理员仍可从 `/fittings?member=<site-account-uuid>` 读取有效绑定成员的个人方案；不能代成员写入游戏。
- 有军团技能管理权限的人，可从军团装配方案生成技能要求。固定 SDE 递归合并舰船、已装装备、弹药、无人机/舰载机的前置技能，同一技能取最高等级。普通货舱备用装备不计入。编辑器支持改名称、等级、增删技能；保存后可立即再编辑，或跳到技能管理。生成的是最低前置要求，不是最优技能推荐，也不修改游戏训练队列。
- `/fittings` 不再提供模拟编辑器、CPU/PG/DPS 面板或属性计算。旧草稿表、兼容 API 与历史引擎源码暂保留，当前页面不加载 WASM/SDE 模拟资源。

## 授权与边界

所有端点受 `fittings.self` 和会话保护，写操作校验 CSRF，后端再次检查对象范围。管理员身份按当前标志核对；未开放可委托的“军团配装写”能力。军团可见范围复用宿主的有效绑定军团、管理员范围和已有军团技能管理范围；技能写入仍由 skills 的 corporation.skills 策略独立检查。

启用 `MODULES` 中的 fittings 时，SSO 在 SeAT 兼容基线 57 项上追加 `esi-fittings.write_fittings.v1`，共 58 项。开发者应用需启用该 scope，已有授权需本人重新授权；页面能识别缺失授权。令牌留在 eve 模块，fittings 仅调用宿主注入的业务服务。

保存请求先短事务登记，再请求 ESI，最后记录结果。账号 UUID request_key 防止跨对象复用；同账号、角色、方案及版本仅保存一次。状态为 sending/saved/failed/unknown；只有确定未成功的 failed 可由用户再次操作。超时、断连或不完整成功响应记 unknown，禁止盲目重发；目前需在游戏核对，不提供自动清除未知记录或强制重试入口。修改后的新版本可单独保存。游戏中删除方案不会自动清除本站幂等记录。

写请求经过共用令牌桶和观测链，但不读取或写入响应缓存。仅 201 且有效 fitting_id 视为成功。缺授权、角色归属变化在请求前拒绝；网络抓取不放进库事务。

## EFT 与静态数据

导入接受官方格式的英文/中文类型名称、空槽位、离线标记、物品数量及装备行弹药。名称长度 1–50，说明最多 500 字，物品 1–512 行。未识别物品拒绝，不悄悄丢弃。若装备行附带弹药但没有数量，需同时在货舱填写 `弹药名称 x数量`；系统不推测装弹数量。游戏方案格式不保留离线状态。该导入不执行 CPU/PG 或完整配装合法性模拟，最终保存仍由游戏验证。

参考文件 `internal/modules/fittings/reference.json` 来自官方 SDE build 3503375，含已发布类型名称、槽位和六组 requiredSkill/level 属性。数据库显示名称仍经 StaticDataService 优先中文解析，列表舰船名称批量查询。参考数据与技能目录固定版本，升级需一起验证，不被数据库名称自动更新替换。

从仓库根目录重新生成：`python scripts/fitting-reference.py <official-sde-jsonl.zip> --build <sde-build>`。尖括号为待替换参数。

## 数据、接口与本地升级

Goose 29 新增 fittings_library、fittings_library_audit、fittings_game_saves；旧 Goose 27 草稿/快照不删除。军团库每团最多 300 个方案。skills 继续独占技能方案 store；EVE 独占凭据、快照、缓存与限流。

端点均以 `/api/v1/fittings/library` 为前缀：

| 方法与路径 | 行为 |
| --- | --- |
| GET /context | 可见军团、本人角色写授权及管理员标志 |
| GET ?corporation_id= | 军团方案列表 |
| POST / | 管理员导入 EFT，含 corporation_id / request_key / eft / description |
| GET /{id} | 读取可见方案 |
| PUT /{id}、DELETE /{id} | 管理员替换/删除，需 corporation_id 和当前 version |
| GET /{id}/requirements | 递归前置技能及 SDE build |
| POST /{id}/save-to-game | version / character_id / request_key；返回状态及 fitting_id |

完整结构见 [OpenAPI](../../api/openapi.yaml)。本站所有 ID/version 为十进制字符串，ESI 上游按官方数值类型发送。冲突 409、隐藏对象/无对象权限 404、输入问题 400；ESI 保存结果通过状态表达。

1. 备份本地数据库和 `.env`，执行 `npm run db:migrate`（Goose 29 + River）。
2. 保留 MODULES=fittings，技能方案入口还需 skills；重启 API，运行 `npm --prefix web run build`。
3. 在开发者应用增加配装写 scope，并由角色本人重新授权。浏览 `/fittings`，管理员可开始导入。

回退保留 29 表；回退到旧同步版本前必须先停止 worker、阻塞新增资源目标并取消旧程序不认识的配装作业，见[生产部署](../deployment.md)。不要为普通回退执行 Down，29 Down 会删除军团库、审计和游戏保存记录。当次生产已升级至 Goose 29（当前统一版本见项目状态），配装/技能模块已启用，7 个角色的三类快照完成同步。用户确认写 scope 已开启，旧角色仍需重新授权；CCP 人机验证和真实游戏写入由玩家完成，自动化未越过挑战。

来源：[CCP ESI OpenAPI](https://esi.evetech.net/meta/openapi.json?compatibility_date=2026-09-14)、[官方配装格式](https://developers.eveonline.com/docs/guides/fitting/)、[官方 SDE](https://developers.eveonline.com/docs/services/static-data/)。布局见[页面记录](../ui/fittings.md)。
