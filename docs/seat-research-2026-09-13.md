# GloryNavy 军团 SeAT 初步调研

> 后续决策：用户已选择 Go 自研，并明确服务器为 4C4G。当前建议见 [自研技术栈](tech-stack.md)。本文保留为 SeAT 方案调研记录，不再作为实施基线。

调研日期：2026-09-13。按 EVE Online 军团管理场景展开；服务器、军团人数和首要业务尚待确认。当前项目目录为空，本次只做资料与选型调研，未安装或部署系统。

## 结论与选型建议

建议先以 SeAT 5 做小规模功能验证，以插件实现军团业务。它已有 ESI 数据同步、角色资料、军团资料及权限管理能力，能够明显减少基础建设工作。正式选型还需通过两项验证：目标服务器的 SSO/ESI 接入，以及底层框架的维护方案。[SeAT 功能介绍](https://developers.eveonline.com/docs/community/seat/)

| 路线 | 适用条件 | 主要投入 | 本项目判断 |
| --- | --- | --- | --- |
| SeAT 5 + 插件 | 成员审核、角色和军团数据是重点 | 部署、权限配置、插件验证、业务定制 | 首选验证对象；生产采用附带维护条件 |
| Alliance Auth + 应用 | 多服务账号、组织分组与访问管理更重要 | 服务接入、应用配置及数据审核应用 | 备选，特别是 Discord 等服务管理占主导时 |
| 自研军团平台 | 有长期产品计划，现成工具难以覆盖主要流程 | 登录、令牌、同步、权限、业务与持续维护 | 当前信息不足以支持直接投入全面自研 |

Alliance Auth 的核心定位包括应用与服务访问管理，也有 Member Audit 提供角色资料审查，因此不能简单认为它只管账号、不管数据。上述排序是结合当前需求的工程判断，并非性能或质量排名。[AA 核心功能](https://allianceauth.readthedocs.io/en/v4.11.2/features/core/index.html)、[Member Audit](https://apps.allianceauth.org/apps/detail/aa-memberaudit)

## 当前版本与维护情况

以下发布信息通过 GitHub Releases API 核对；各仓库独立发版，不能只凭主项目的版本日期判断整个系统是否仍在维护。

| 仓库 | 查询到的最新 Release | 发布日期（UTC） |
| --- | --- | --- |
| eveseat/seat | 5.0.1 | 2024-05-20 |
| eveseat/eveapi | 5.0.37 | 2026-08-09 |
| eveseat/web | 5.0.35 | 2026-05-05 |
| eveseat/seat-docker | 5.0.95 | 2025-12-31 |

依据：[主项目发布](https://github.com/eveseat/seat/releases/tag/5.0.1)、[数据组件发布](https://github.com/eveseat/eveapi/releases/tag/5.0.37)、[Web 组件发布](https://github.com/eveseat/web/releases/tag/5.0.35)、[Docker 项目发布](https://github.com/eveseat/seat-docker/releases/tag/5.0.95)。Release 标签不等同于当前容器内所有 Composer 包的版本。

一个需要影响生产决策的事实：主项目 5.0.1 及本次读取的 master 均声明 `laravel/framework: ^10.0`；Laravel 10 的官方安全维护已于 2025-02-04 结束。组件仍有更新，不能消除这个框架维护问题。正式上线前应核查实际安装依赖、上游升级计划及可用的补丁维护方式；本次没有执行依赖漏洞扫描，也没有证据断言存在某个可利用漏洞。[SeAT 依赖文件](https://github.com/eveseat/seat/blob/5.0.1/composer.json)、[Laravel 官方支持周期](https://laravel.com/docs/10.x/releases)

版本资料存在差异：SeAT 5 手动安装文档使用 PHP 8.2；当前 Dockerfile 已使用 PHP 8.4；主项目的 Composer 最低约束又是 `^8.1`。实现时应固定并验证一组镜像与依赖版本，而不是从不同年代教程拼装环境。[手动安装](https://eveseat.github.io/docs/installation/manual_installation/)、[当前 Dockerfile](https://github.com/eveseat/seat-docker/blob/master/Dockerfile)

## 军团功能与实现来源

| 需求 | 可复用部分 | 我们需要补充或验证的内容 |
| --- | --- | --- |
| 成员登记、角色绑定 | SeAT SSO、角色资料 | 主角色规则、绑定引导、授权失效提示 |
| 入团审核 | cryocaustik/seat-hr | 中文问卷、审核分工、拒绝原因与历史留痕 |
| 军团与角色数据 | SeAT 核心 ESI 同步 | 实际 scopes、军团职务要求、可见范围 |
| 标准配船、技能匹配 | cryptatech/seat-fitting | 本团 doctrine、配船版本和技能标准 |
| 舰船补损 SRP | cryptatech/seat-srp | 活动关联、重复申请处理、审核与付款登记规则 |
| 舰队出勤 | 需要进一步验证插件或定制 | 活动、签到/名单快照、FC 确认、按玩家合并小号、人工修正记录 |
| Discord 角色同步 | warlof/seat-discord-connector | 连接器依赖、分组映射和离团权限回收 |
| 回购、税务、工业、建筑 | 社区插件目录已有候选 | 在明确首要需求后，逐个验证数据与结算口径 |

SeAT 核心提供分级权限和基于 API 数据的角色自动化；中文翻译已有基础，但各语言完成度不同，不能据此保证插件和业务流程完整中文化。[SeAT 功能与本地化](https://developers.eveonline.com/docs/community/seat/)

本次核对了以下插件仓库及默认分支的依赖声明，尚未安装测试。仓库提交时间只是维护线索，不代表质量保证。

| Composer 包名 | 仓库 | 最近推送日期（UTC） | 已确认事项 |
| --- | --- | --- | --- |
| cryocaustik/seat-hr | cryocaustik/seat-hr | 2025-03-06 | README 明确 v2 分支用于 SeAT 5；Intel 仍标注 coming soon |
| cryptatech/seat-srp | eveseat-plugins/seat-srp | 2026-02-22 | 依赖 SeAT 5；需要至少一个价格提供器 |
| cryptatech/seat-fitting | eveseat-plugins/seat-fitting | 2025-12-23 | 依赖 SeAT 5；提供配船与角色技能比较 |
| warlof/seat-discord-connector | zenobio93/seat-discord-connector | 2025-04-06 | 6.0.x 分支依赖 SeAT 5 及 seat-connector ^3.0 |

依据：[HR](https://github.com/cryocaustik/seat-hr)、[SRP](https://github.com/eveseat-plugins/seat-srp)、[配船](https://github.com/eveseat-plugins/seat-fitting)、[Discord 连接器](https://github.com/zenobio93/seat-discord-connector/tree/6.0.x)。插件名称、仓库归属及插件自身主版本号不一定与 SeAT 主版本一致，安装时应核对 Composer 包名和依赖。

回购、工业、税费、建筑燃料等可从官方社区目录继续筛选；此处只确认存在候选，没有把目录中的所有插件都认定为可直接上线。[社区插件目录](https://eveseat.github.io/docs/community_packages/)

## 数据接入与业务边界

国际服的标准接入路径是注册 EVE 第三方应用，配置 Client ID、Client Secret 和回调地址，然后由成员通过 SSO 授权访问其角色数据。SSO 授权以角色及 scopes 为边界，不应承诺一次绑定就能自动识别玩家所有未授权角色。[SeAT ESI 配置](https://eveseat.github.io/docs/configuration/esi_configuration/)、[EVE SSO](https://developers.eveonline.com/docs/services/sso/)

国服仍是待验证条件。本次没有获得足够的一手资料，证明其当前第三方应用注册、回调和长期令牌刷新与国际服方案等价；尝试读取国服 ESI UI 也未成功。这不等于判定国服没有接口。若目标为国服，应优先拿到现行开发者文档或可用授权流程，完成一个角色的数据读取和刷新验证，再决定 SeAT 的适配方式。

建议的产品规则：

- 把“游戏角色”“本站用户/玩家”“军团身份”分开；允许一个玩家绑定多个角色，出勤按规则合并。
- 出勤作为独立业务记录保存。报名、在舰队中出现、参与战斗和符合有效出勤条件是不同证据；由 FC 确认并保留修正记录。
- 补损采用“申请 → 审核 → 待支付 → 已支付/拒绝”；首版按人工游戏内付款与站内登记设计，不预设自动转账能力。
- 对授权撤销、临时接口故障、确认离团使用不同状态；避免一次同步失败直接触发踢人或永久删除权限。
- 展示数据最近同步时间和失败状态，不把缓存数据呈现为实时状态。
- 军团私有接口逐项验证授权角色、scope 和游戏内职务；普通成员授权不能被当作军团管理权限。

ESI 在 2025–2026 年持续调整限流和缓存机制。当前文档说明部分端点使用分组限流，超限返回 429 与 Retry-After，另有旧错误限流机制。因此同步必须遵循响应头、缓存有效期、错峰调度与失败退避；新增同步逻辑还应关注 X-Compatibility-Date。采用 SeAT 后仍要通过运行验证确认已安装版本对这些机制的支持。[当前限流文档](https://developers.eveonline.com/docs/services/esi/rate-limiting/)、[ESI 版本兼容机制](https://developers.eveonline.com/docs/services/esi/overview/)

## 建议的第一版范围

默认先围绕人员管理建立完整流程，待军团确认优先级后调整：

1. 成员 SSO 登录、主角色选择、多角色绑定和授权状态。
2. 军团成员名单、待审核申请、审核备注。
3. 普通成员、人事、FC、财务、管理员的最小权限划分。
4. 中文成员首页：待办、公告、配船入口及数据更新时间。
5. 一个业务闭环：优先在出勤和补损中选择一个完成试用。

资产财务总览、工业订单、矿税、回购和复杂贡献积分列为后续模块。选择标准应是军团当下最耗人工的流程，而不是插件数量。

定制建议作为独立 SeAT 插件维护，自有表记录审核、活动、出勤和补损规则，避免直接改 vendor 文件。第一阶段优先沿用现有服务端页面；是否独立建设前端，留给实际使用反馈决定。[SeAT 插件开发与升级约束](https://eveseat.github.io/docs/developer_guides/updating_plugins/)

## 部署与验证建议

官方推荐 Docker；当前 Compose 将前台、后台 worker、定时调度器与 Redis 分开，数据库通过 MariaDB 配置文件加入。由此建议采用 Linux 主机、Docker Compose、HTTPS 反向代理，以及数据库和持久化目录备份。[Docker 安装指南](https://eveseat.github.io/docs/installation/docker_installation/)、[Compose 源码](https://github.com/eveseat/seat-docker/blob/master/docker-compose.yml)、[MariaDB 配置](https://github.com/eveseat/seat-docker/blob/master/docker-compose.mariadb.yml)

若仅供少量成员试用，可把 2–4 vCPU、4–8 GB 内存、60–100 GB SSD 作为初始估算，再根据同步角色数、资产量、保留期和队列积压调整。这不是官方最低配置或经过压测的容量承诺。人数和托管地区尚未明确，本次不提供未经核实的云主机月费。

验证按顺序进行：

1. 确认服务器和可用 SSO/ESI，使用少量自愿测试角色完成登录、刷新、撤销与重新授权。
2. 固定镜像与依赖版本，检查框架维护方案，并验证队列、调度、SDE 和数据同步。
3. 先装最少插件，验证普通成员无法读取他人敏感资料，以及人事、FC、财务之间的权限隔离。
4. 演练一条入团流程和选定业务流程，测试重复提交、接口失败、角色离团及授权失效。
5. 完成一次从备份恢复；明确数据库、持久化文件、配置密钥的恢复方式，然后扩大试用。

目前最有价值的补充信息是：国际服还是国服、活跃玩家数与绑定角色数、优先业务、使用的聊天/语音平台、是否已有域名和 Linux 服务器。
