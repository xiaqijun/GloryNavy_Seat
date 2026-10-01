# 游戏术语来源检查（2026-09-20）

**后续修正已完成并发布生产（`v0.1.0-terminology-20260920`）**：进一步查到官方 SDE 的 accountingEntryTypes / corporationRoles 及同版本 Tranquility 客户端本地化原文，已替换对应手写映射、统一槽位/币名并纠正 Absolution。当前来源与验证见[术语指南](../integrations/eve-terminology.zh-CN.md)。下文及附录保留修正前的审查和旧词，不代表当前运行时仍使用这些译名；“尚未核验”是当时状态。

## 结论与范围

项目存在未经逐条官方中文核验的手写游戏术语映射，ESS 不是唯一一项。本轮检查前端模块的标签映射、中文至英文词表、服务端名称解析和静态目录生成脚本，并将三个生成目录与本地 CCP Tranquility SDE build 3503375 原包逐项比对。

**手写不等于错误，枚举来自官方也不等于中文标签来自官方。** 以下“待核对”表示仓库尚无逐条客户端中文/官方本地化来源证据，不表示全部译错。未读取当前游戏客户端的中文本地化文件，不能宣称已经完成所有术语的官方译名校准。本轮仅审查及完善文档，没有替换运行时文案或发布生产。

## 待核对的游戏标签

| 范围 | 源码 | 实现与风险 |
| --- | --- | --- |
| 钱包流水 | [labels.ts](../../web/src/modules/wallet/labels.ts) | 162 项 `journalTypes` 都是手写 `msg` 映射。包括 `ess_escrow_transfer` → ESS 托管转账、`market_escrow` → 市场托管、`courier_mission_escrow` → 快递任务托管、`factory_slot_rental_fee` → 工厂槽位租金、`security_processing_fee` → 安全处理费。没有逐条官方中文来源；整个表需要核验，不能只改 ESS。 |
| 钱包关联对象 | [labels.ts](../../web/src/modules/wallet/labels.ts) | 12 项 `contextTypes` 是本站对协议字段的解释，如 `eve_system` → EVE 系统、`industry_job_id` → 工业任务。应区分解释标签和游戏内正式名称。 |
| 军团职务 | [role-labels.ts](../../web/src/modules/access/role-labels.ts) | 26 项固定职务和 4 类分部模板（每类 1–7）均为手写。优先核对 `Config_Starbase_Equipment` → 母星基地设备配置、`Starbase_Defense_Operator` → 母星基地防御操作员、`Deliveries_Container_Take` → 取出交付容器、`Fitting_Manager` → 装配经理等。未确认替代词前不猜译。 |
| 合同类型/状态 | [contracts-api.ts](../../web/src/modules/eve/contracts-api.ts) | 5 个类型、10 个状态采用本地映射；例如 `courier` → 快递运输、`outstanding` → 未完成、`reversed` → 已逆转。官方枚举及状态逻辑与游戏中文文案是两项不同检查；此前枚举核对不构成中文来源证据。`loan` 标签中的“历史”是本站提示。 |
| 槽位与物品位置 | [model.ts](../../web/src/modules/fittings/model.ts)、[library-page.tsx](../../web/src/modules/fittings/library-page.tsx)、[battle-panel.tsx](../../web/src/modules/attendance/battle-panel.tsx) | 高/中/低槽、改装件、子系统、货舱等手写重复映射，不由 SDE 名称生成器维护。发现 FighterBay 跨页不一致，见下节。 |
| 旧福利项目与旗舰称呼 | [welfare/api.ts](../../web/src/modules/welfare/api.ts) | “毒蜥、伊什塔、洛基、救世、普通旗舰、大航”等是旧项目名称/简称，不能作为物品或舰船类别的官方全称。新的成长项目来自军团方案，方案名称属于用户内容；两者不能混淆。 |
| 旧模拟统计代码 | [statistics.ts](../../web/src/modules/fittings/statistics.ts) | 能量栅格、改装校准值、炮台安装位等也属手写显示文案。模拟页面已退出当前产品入口，单列为遗留代码，不报告成当前上线模拟功能。 |

## 已确认的跨页不一致

1. `FighterBay`：配装的 `model.ts:53`、`library-page.tsx:392` 使用“铁骑舰载机挂舱”；出勤的 `battle-panel.tsx:236` 使用“铁骑舰载机舱”。英文词表均译为 `Fighter bay`。当前配装详情直接复用 `slotNames`，因此不是只有遗留模拟代码受影响。
2. 果壳币：英文词表中独立名称/兑换动作使用 `Nutshell Coins`，流水、价值、福利和合并文案使用 `Shell Coins`。这是本站货币命名不统一，不属于 EVE 官方术语问题。需统一项目自己的英文名称。

源码：[英文词表](../../web/src/lib/locales/en.ts)。本轮仅确认不一致，没有选择新的官方中文替代词。

## 已有可靠来源的名称

| 数据 | 本轮证据 | 结果 |
| --- | --- | --- |
| 技能目录及组名 | [生成器](../../scripts/skill-catalog.py)、[目录](../../internal/modules/skills/catalog.json)，对照本地官方 ZIP 的 types/groups | 511 项名称、英文名、组中英文名逐项一致，0 处差异 |
| 配装参考中的物品/船型/技能及组名 | [生成器](../../scripts/fitting-reference.py)、[目录](../../internal/modules/fittings/reference.json)，对照同一 ZIP | 7,885 项名称及组中英文名逐项一致，0 处差异 |
| 配装 UI 分类目录 | [生成器](../../scripts/fitting-group-names.py)、[目录](../../web/src/modules/fittings/group-names.json)，对照 groups | 540 项中文或缺中文时的英文回退逐项一致，0 处差异 |
| 动态物品、技能、船型和星系名称 | [StaticDataService](../../internal/modules/eve/sde.go)、[SDE 指南](../integrations/sde-names.zh-CN.md) | 代码使用活动 SDE 对应语言及回退链路，无按英文临时直译；本轮检查链路，没有扫描生产数据库每条名称 |
| 玩家/军团/方案名称、备注、合同描述 | [语言约定](language.md)、[合同摘要](../../internal/modules/eve/contract_summary.go) | 原文保留；空描述生成摘要是本站展示逻辑，摘要中的物品名仍经 SDE，不冒充游戏填写的合同描述 |

比较基准为忽略目录 `.local/sde/eve-online-static-data-3503375-jsonl.zip`，不代表 3503375 是最新 SDE，也不等于核验当前客户端每个界面文字。上述目录可能相互包含，数量不能相加成去重游戏名总数。

## 不应误判为游戏直译的内容

同步状态、权限管理动作、福利审批状态、PAP、补损推荐提示、训练完成等待确认、买/中/卖估价标签属于本站功能文案。合同出售/求购方向也由本站逻辑推导。这些需要中英文质量和一致性检查，但不要求伪造一个“官方游戏原文”。`internal/platform/locale/messages.en.json` 是本站固定错误消息的英文映射，不是官方 EVE 本地化表。

## 后续修正顺序

1. 钱包 162 项及职务表逐条登记稳定代码、客户端正式名称、语言、证据来源/版本和核验状态。优先 ESS、escrow、starbase、deliveries 等直译风险词。
2. 再核对合同及槽位标签。没有可靠中文来源时保留原始代码/已有英文作为核对依据，不将新的推测译法写成官方名称。
3. 公共游戏术语统一维护，消费页面不各自复制；本站币名独立统一。已经来自 SDE 的名称继续走 SDE，不转入手写词表。
4. 文案测试只能保证映射与回退行为；例如钱包测试断言“ESS 托管转账”，并不能证明这个中文正确。官方来源核验必须独立记录。

完整的静态钱包、关联对象、固定职务、合同类型/状态映射附后，均为**当前项目标签，尚未逐条核验官方中文**。

## 附录：钱包流水（162 项）

| 原始代码 | 当前项目中文（待官方核验） |
| --- | --- |
| `acceleration_gate_fee` | 加速轨道费用 |
| `achievement_category_milestone_reward` | 成就分类里程碑奖励 |
| `achievement_milestone_reward` | 成就里程碑奖励 |
| `advertisement_listing_fee` | 广告刊登费 |
| `agent_donation` | 代理人捐款 |
| `agent_location_services` | 代理人定位服务 |
| `agent_miscellaneous` | 代理人其他费用 |
| `agent_mission_collateral_paid` | 任务保证金支付 |
| `agent_mission_collateral_refunded` | 任务保证金退回 |
| `agent_mission_reward` | 任务奖励 |
| `agent_mission_reward_corporation_tax` | 任务奖励军团税 |
| `agent_mission_security_tax` | 任务安全税 |
| `agent_mission_time_bonus_reward` | 任务限时奖励 |
| `agent_mission_time_bonus_reward_corporation_tax` | 任务限时奖励军团税 |
| `agent_security_services` | 代理人安全服务 |
| `agent_services_rendered` | 代理人服务费用 |
| `agents_preward` | 代理人预付奖励 |
| `air_career_program_reward` | AIR 职业计划奖励 |
| `alliance_maintainance_fee` | 联盟维护费 |
| `alliance_registration_fee` | 联盟注册费 |
| `allignment_based_gate_toll` | 阵营通行费 |
| `asset_safety_recovery_tax` | 资产安全取回税 |
| `bounty` | 悬赏 |
| `bounty_prize` | 悬赏奖励 |
| `bounty_prize_corporation_tax` | 悬赏奖励军团税 |
| `bounty_prizes` | 悬赏奖励 |
| `bounty_reimbursement` | 悬赏退还 |
| `bounty_surcharge` | 悬赏附加费 |
| `brokers_fee` | 经纪人费 |
| `campaign_objective_isk_reward` | 战役目标 ISK 奖励 |
| `clone_activation` | 克隆激活 |
| `clone_transfer` | 克隆转移 |
| `contraband_fine` | 违禁品罚款 |
| `contract_auction_bid` | 合同拍卖出价 |
| `contract_auction_bid_corp` | 军团合同拍卖出价 |
| `contract_auction_bid_refund` | 合同拍卖出价退回 |
| `contract_auction_sold` | 合同拍卖成交 |
| `contract_brokers_fee` | 合同经纪人费 |
| `contract_brokers_fee_corp` | 军团合同经纪人费 |
| `contract_collateral` | 合同保证金 |
| `contract_collateral_deposited_corp` | 军团合同保证金存入 |
| `contract_collateral_payout` | 合同保证金赔付 |
| `contract_collateral_refund` | 合同保证金退回 |
| `contract_deposit` | 合同押金 |
| `contract_deposit_corp` | 军团合同押金 |
| `contract_deposit_refund` | 合同押金退回 |
| `contract_deposit_sales_tax` | 合同押金销售税 |
| `contract_price` | 合同价款 |
| `contract_price_payment_corp` | 军团合同价款支付 |
| `contract_reversal` | 合同冲正 |
| `contract_reward` | 合同奖励 |
| `contract_reward_deposited` | 合同奖励存入 |
| `contract_reward_deposited_corp` | 军团合同奖励存入 |
| `contract_reward_refund` | 合同奖励退回 |
| `contract_sales_tax` | 合同销售税 |
| `copying` | 蓝图复制 |
| `corporate_reward_payout` | 军团奖励发放 |
| `corporate_reward_tax` | 军团奖励税 |
| `corporation_account_withdrawal` | 军团账户支取 |
| `corporation_bulk_payment` | 军团批量付款 |
| `corporation_dividend_payment` | 军团分红 |
| `corporation_liquidation` | 军团清算 |
| `corporation_logo_change_cost` | 军团标志变更费 |
| `corporation_payment` | 军团付款 |
| `corporation_registration_fee` | 军团注册费 |
| `cosmetic_market_component_item_purchase` | 涂装市场组件购买 |
| `cosmetic_market_skin_purchase` | 涂装市场涂装购买 |
| `cosmetic_market_skin_sale` | 涂装市场涂装出售 |
| `cosmetic_market_skin_sale_broker_fee` | 涂装出售经纪人费 |
| `cosmetic_market_skin_sale_tax` | 涂装出售税 |
| `cosmetic_market_skin_transaction` | 涂装市场交易 |
| `courier_mission_escrow` | 快递任务托管 |
| `cspa` | CSPA 通信费用 |
| `cspaofflinerefund` | CSPA 离线退费 |
| `daily_challenge_reward` | 每日挑战奖励 |
| `daily_goal_payouts` | 每日目标奖励 |
| `daily_goal_payouts_tax` | 每日目标奖励税 |
| `datacore_fee` | 数据核心费用 |
| `dna_modification_fee` | DNA 修改费 |
| `docking_fee` | 停靠费用 |
| `duel_wager_escrow` | 决斗赌注托管 |
| `duel_wager_payment` | 决斗赌注支付 |
| `duel_wager_refund` | 决斗赌注退回 |
| `ess_escrow_transfer` | ESS 托管转账 |
| `external_trade_delivery` | 外部交易交付 |
| `external_trade_freeze` | 外部交易冻结 |
| `external_trade_thaw` | 外部交易解冻 |
| `factory_slot_rental_fee` | 工厂槽位租金 |
| `flux_payout` | 超网奖金发放 |
| `flux_tax` | 超网税 |
| `flux_ticket_repayment` | 超网节点退款 |
| `flux_ticket_sale` | 超网节点销售 |
| `freelance_jobs_broadcasting_fee` | 自由职业任务广播费 |
| `freelance_jobs_duration_fee` | 自由职业任务时长费 |
| `freelance_jobs_escrow_refund` | 自由职业任务托管退回 |
| `freelance_jobs_reward` | 自由职业任务奖励 |
| `freelance_jobs_reward_corporation_tax` | 自由职业任务奖励军团税 |
| `freelance_jobs_reward_escrow` | 自由职业任务奖励托管 |
| `gm_cash_transfer` | GM 资金转账 |
| `gm_plex_fee_refund` | GM PLEX 费用退回 |
| `industry_job_tax` | 工业任务税 |
| `industry_security_tax` | 工业安全税 |
| `infrastructure_hub_maintenance` | 基础设施中心维护费 |
| `inheritance` | 继承款项 |
| `insurance` | 保险 |
| `insurgency_corruption_contribution_reward` | 叛乱腐化贡献奖励 |
| `insurgency_suppression_contribution_reward` | 叛乱镇压贡献奖励 |
| `item_trader_payment` | 物品交易商付款 |
| `jump_clone_activation_fee` | 远距克隆激活费 |
| `jump_clone_installation_fee` | 远距克隆安装费 |
| `kill_right_fee` | 击杀权费用 |
| `lp_store` | 忠诚点商店 |
| `manufacturing` | 制造 |
| `market_escrow` | 市场托管 |
| `market_fine_paid` | 市场罚款 |
| `market_provider_tax` | 市场服务商税 |
| `market_security_tax` | 市场安全税 |
| `market_transaction` | 市场交易 |
| `medal_creation` | 勋章创建 |
| `medal_issued` | 勋章颁发 |
| `milestone_reward_payment` | 里程碑奖励支付 |
| `mission_completion` | 任务完成 |
| `mission_cost` | 任务成本 |
| `mission_expiration` | 任务到期 |
| `mission_reward` | 任务奖励 |
| `npc_bounty_security_tax` | NPC 悬赏安全税 |
| `office_rental_fee` | 办公室租金 |
| `operation_bonus` | 行动奖金 |
| `opportunity_reward` | 机遇奖励 |
| `planetary_construction` | 行星建设 |
| `planetary_export_tax` | 行星出口税 |
| `planetary_import_tax` | 行星进口税 |
| `player_donation` | 玩家转账 |
| `player_trading` | 玩家交易 |
| `project_discovery_reward` | 探索计划奖励 |
| `project_discovery_tax` | 探索计划税 |
| `project_payouts` | 项目奖励发放 |
| `reaction` | 反应 |
| `redeemed_isk_token` | ISK 代币兑换 |
| `release_of_impounded_property` | 扣押资产取回 |
| `repair_bill` | 维修费用 |
| `reprocessing_tax` | 提炼税 |
| `researching_material_productivity` | 材料效率研究 |
| `researching_technology` | 科技研究 |
| `researching_time_productivity` | 时间效率研究 |
| `resource_wars_reward` | 资源战争奖励 |
| `reverse_engineering` | 逆向工程 |
| `season_challenge_reward` | 赛季挑战奖励 |
| `security_processing_fee` | 安全处理费 |
| `shares` | 股份 |
| `skill_purchase` | 技能购买 |
| `skyhook_claim_fee` | 轨道天钩认领费 |
| `sovereignity_bill` | 主权账单 |
| `store_purchase` | 商店购买 |
| `store_purchase_refund` | 商店购买退款 |
| `structure_gate_jump` | 建筑跳跃通行费 |
| `transaction_tax` | 交易税 |
| `under_construction` | 建设中 |
| `upkeep_adjustment_fee` | 维护调整费 |
| `war_ally_contract` | 战争盟友合同 |
| `war_fee` | 宣战费 |
| `war_fee_surrender` | 战争投降费 |


## 附录：钱包关联对象（12 项）

| 原始代码 | 当前项目中文（待官方核验） |
| --- | --- |
| `structure_id` | 建筑 |
| `station_id` | 空间站 |
| `market_transaction_id` | 市场交易 |
| `character_id` | 角色 |
| `corporation_id` | 军团 |
| `alliance_id` | 联盟 |
| `eve_system` | EVE 系统 |
| `industry_job_id` | 工业任务 |
| `contract_id` | 合同 |
| `planet_id` | 行星 |
| `system_id` | 星系 |
| `type_id` | 物品类型 |


## 附录：固定军团职务（26 项）

| 原始代码 | 当前项目中文（待官方核验） |
| --- | --- |
| `Director` | 总监 |
| `Accountant` | 会计 |
| `Junior_Accountant` | 初级会计 |
| `Auditor` | 审计员 |
| `Personnel_Manager` | 人事经理 |
| `Security_Officer` | 安全官 |
| `Contract_Manager` | 合同经理 |
| `Diplomat` | 外交官 |
| `Trader` | 交易员 |
| `Project_Manager` | 项目经理 |
| `Factory_Manager` | 工厂经理 |
| `Station_Manager` | 空间站经理 |
| `Fitting_Manager` | 装配经理 |
| `Brand_Manager` | 品牌经理 |
| `Communications_Officer` | 通讯官 |
| `Skill_Plan_Manager` | 技能计划经理 |
| `Config_Equipment` | 设备配置 |
| `Config_Starbase_Equipment` | 母星基地设备配置 |
| `Starbase_Defense_Operator` | 母星基地防御操作员 |
| `Starbase_Fuel_Technician` | 母星基地燃料管理员 |
| `Rent_Factory_Facility` | 租用工厂 |
| `Rent_Research_Facility` | 租用研究设施 |
| `Rent_Office` | 租用办公室 |
| `Deliveries_Container_Take` | 取出交付容器 |
| `Deliveries_Query` | 查看交付物品 |
| `Deliveries_Take` | 取出交付物品 |


## 附录：合同类型（5 项）

| 原始代码 | 当前项目中文（待官方核验） |
| --- | --- |
| `item_exchange` | 物品交换 |
| `courier` | 快递运输 |
| `auction` | 拍卖 |
| `loan` | 借贷（历史） |
| `unknown` | 未知类型 |


## 附录：合同状态（10 项）

| 原始代码 | 当前项目中文（待官方核验） |
| --- | --- |
| `outstanding` | 未完成 |
| `in_progress` | 进行中 |
| `finished_issuer` | 发起方已完成 |
| `finished_contractor` | 接受方已完成 |
| `finished` | 已完成 |
| `cancelled` | 已取消 |
| `rejected` | 已拒绝 |
| `failed` | 已失败 |
| `deleted` | 已删除 |
| `reversed` | 已逆转 |
