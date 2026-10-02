# EVE 官方术语目录

> 状态索引（2026-09-23）：本指南对应的业务已上线；未交付的扩展与真实验收缺口见[待办](../backlog.md)，当前版本见[项目状态](../project-status.md)。下文带日期/Goose 的发布前记录保留当时语境，不能据此认定当前未发布；升级执行仓库最新迁移，不停在模块初始版本。

2026-09-20，已随前端 `v0.1.0-terminology-20260920` 发布生产，验收见[项目状态](../project-status.md)。源头核对纠正了[此前审查](../ui/game-terminology-audit-2026-09-20.md)的遗漏：SDE 不止有物品名称，`accountingEntryTypes.jsonl` 和 `corporationRoles.jsonl` 已包含中英文显示名；合同与槽位可以从同版本 Tranquility 客户端本地化资源核验。名称对应 build 3503375，不声称自动跟随未来客户端更新。

## 数据与边界

| 域 | 数量 | 名称来源 |
| --- | --- | --- |
| 钱包流水 | 162 | SDE accountingEntryTypes，ESI 原始代码精确匹配 `internalName`，中英文均取 `name` |
| 军团职务 | 55 | SDE corporationRoles，通过明确的 `shortName` 对照连接 ESI 代码，含 28 个分部职务与地面后勤官 |
| 合同 | 5 类型 / 10 状态 | 客户端 `UI/Contracts/ContractsWindow/*`；未知类型采用 `UI/Generic/Unknown` |
| 装备位置 | 10 | 客户端 Ship、InfoWindow、Killmails、Common 标签；配装及出勤共用 |
| 钱包关联对象 | 12 | 10 个有客户端对应名；`eve_system` / `industry_job_id` 是协议上下文，保留英文技术标签 EVE system / Industry job，不标注为官方游戏中文 |
| 旧成长项目船型 | 4 | SDE type 17715 / 12005 / 29990 / 22448 的正式全名；仅作用于无项目自定义名称时的后备显示 |
| 旗舰类别 / 旧统计标签 | 2 / 8 | 客户端 Industry、Fitting、Common 标签；不恢复已经下线的模拟入口 |

关键修正：ESS 为“事件监测装置保证金支付 / ESS Escrow Payment”；`market_escrow` 等同样由官方数据生成，不继续逐条直译。职务如 Personnel Manager 为“人事主管”，Fitting Manager 为“装配主管”，Starbase Fuel Technician 为“母星燃料技术员”。Absolution 正式名称为“救赎级”，旧项目后备名“救世”已纠正。玩家自定义方案/项目名和历史证据不被改写。

合同状态对应关系同时核对同版本客户端 `contractscommon.GetContractStatusText` 的常量与标签引用：`finished_issuer` → ItemsNotYetClaimed（物品未被认领），`finished_contractor` → UnclaimedBySeller（卖方尚未领取货款），`finished` → Finished（已结束），`reversed` → Reversal（撤销）。前两项仍是单方完成，不因文字变化改成全部完成；筛选、导出、状态样式及服务端判断继续使用 ESI 代码。借贷的历史兼容性由现有筛选分组表达，不写进官方名称。

本站果壳币统一为 `Nutshell Coin` / `Nutshell Coins`，属于项目自定义名称。福利审批、PAP、估价方向及同步状态也是本站文案，不套用游戏状态词。

## 生成与更新

- [来源键映射](../../scripts/eve-terminology-sources.json)只维护协议代码到 SDE 记录/客户端标签的关联，不维护手译中文。
- [生成器](../../scripts/eve-terminology.py)验证客户端服务器为 Tranquility、SDE 与客户端 build 一致；按 resfileindex 核对资源长度和 MD5，并记录 SHA256。pickle 只允许基础数据结构，禁止执行全局对象及持久引用，不运行游戏代码。
- [生成目录](../../web/src/lib/eve-terminology.json)保存中英文、来源文件/记录 ID 或本地化标签/message ID，以及源文件摘要。不提交完整客户端、原始 pickle 或本机安装路径。
- [显示服务](../../web/src/lib/eve-terminology.ts)按领域/原始代码和界面语言取值，缺中文回退已有英文，未知代码原样保留；不经本站中文文案词表再次翻译，避免同词不同语义串用。共享 `assetSlotLabel` 保留槽位编号和未知标志。

生成命令（用本机实际路径替换占位符）：

```sh
python scripts/eve-terminology.py <official-jsonl.zip> --client <Tranquility-tq-directory> --resources <ResFiles-directory>
python scripts/eve-terminology.py <official-jsonl.zip> --client <Tranquility-tq-directory> --resources <ResFiles-directory> --check
```

`--check` 逐项重生成并比较整个产物，用于证明名称未偏离源文件；单测只验证语言选择、回退及代码/槽位边界，不能独立证明官方翻译正确。更新时必须审查来源映射，重新运行 `--check`、前端测试和构建，核对钱包/职务/合同/配装相关界面。不在运行中的前端加载客户端资源，不额外请求 ESI。

这是构建期小型术语目录，**不扩展数据库 SDE 导入范围**，不改变现有 SDE 自动更新或版本固定机制。无需迁移、scope 或重新授权。部署仅需匹配前端产物；本次生产发布保留原后端与数据库版本。

本轮验证：完整前端 83 项单测、lint、构建、生成器逐项检查和服务端消息一致性检查通过。钱包/合同/职务共 10 项桌面及手机专项通过（职务用例更新旧译名断言后复测）；核对 ESS 中英文、原始代码、长标签、合同原始状态与筛选、职务折叠和授权变化。不是全量浏览器套件验收。名称及资源版权归 CCP，见[来源声明](../third-party-notices.md)。

[English](eve-terminology.en.md)
