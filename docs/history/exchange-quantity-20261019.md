# 同一奖励多份兑换 · 2026-10-19

用户重新选择后明确只需同一种奖励兑换多份，不包含不同商品购物车。实现说明见[兑换指南](../integrations/exchange.zh-CN.md)。Goose 102；旧订单、报价/内容版本、管理员取消、绑定/对象权限、合同占用边界保留。

GitNexus 预编辑分析：ClaimReward/DecideOrder/RewardOrders 各一处 HTTP 调用（LOW），ClaimForm 由 RewardsPanel 调用；DecisionForm 由兑换页及审批详情共用。physicalValid 的预评估为 HIGH（4 处直接调用，涉及核价/奖励库/福利引用/结算），最终保留原函数和单份规则，另建订单快照校验。共享 MatchRewardDelivery 改读允许整单累计量的校验，须覆盖福利回归。SettlementReward 经宿主注入合并结算，索引未解析调用，已源码确认。索引落后 15 次提交，刷新无进展后中止，空边不视作无影响。

## 验证

- TypeScript/Vite 构建、兑换数量与词表 8 项单测、受影响前端 lint 通过；OpenAPI 经 YAML 解析，sqlc 再生成与已测源码一致，文档相对链接已核对。
- 隔离 PostgreSQL 首轮 exchange 全量 race（314 秒）仅一条旧测试仍把正常兑换当作待审批而失败；现有业务正确进入待发放，修正该测试预期。最终数量/审批降级 race（81.6 秒）、福利成长/统一交付/合并结算（98.5 秒）、宿主权限/CSRF/元数据（23.7 秒）通过；go vet 单独完成。首轮其余兑换、定价、退款与权限用例通过。脚本末尾 Windows 换行曾导致检查退出码非零，确认测试已通过并独立重跑 go vet，临时数据库已清理。
- 新增中英文三尺寸（1280/375/320px）24 项浏览器流程通过；删除重复的接收角色标签后，对最终构建复跑 6 项完整兑换/总量/合同信息流程通过。原有取消/余额回归 6 项通过；原角色选择测试仍断言原生 select 的 option 在 DOM 中可见，按当前控件改用 selectOption，最终三尺寸 3 项复验通过。
- 覆盖省略份数/显式 1 的旧指纹、两份币价逐份取整、越界/余额不足、并发防超卖、失败回滚、退款幂等、奖励内容换版不补新库存、冻结配装/物品/ISK 总量、单份现金不误完成多份订单、整单合同幂等核验、审批详情/投影和合并结算读取。
- 已检查桌面及 320px 截图：[桌面](../ui/reviews/exchange/quantity-desktop.png)、[窄屏](../ui/reviews/exchange/quantity-narrow.png)。均使用浏览器夹具，不是真实兑换或交付。

GitNexus 暂存及 main 比较为 CRITICAL：37 文件、61 个已索引变更、28 条流程，涉及原兑换写入、退款、核价和审批读取。已人工核对仅预期范围，保留奖励库/核价单份限制及权限边界，新增快照校验由上述回归覆盖。发布情况见下。

## 发布

前后端 `v0.1.0-exchange-quantity-20261019` 来源干净且推送的 main `773fd6370f9b9f8550ac1dc3a9f60c19785da930`，Goose 102。前端 143 个文件与最终测试产物一致；729 个后端相关文件与隔离测试目录、生产构建目录逐文件校验一致。按原 activate-release/activate-edge-release 发布，未改配置、代理或业务记录。

备份 `/var/backups/glorynavy/before-v0.1.0-exchange-quantity-20261019-20261019T090136Z.dump`，261757601 字节，同前缀 env/meta.json 保留；未做恢复演练。线上 active/running/ready 200、NRestarts=0，首页/兑换/奖励库/审批页面 200，兑换上下文/奖励/订单/合同信息及审批匿名 401；完整文件清单、HTML/主包/兑换包字节匹配，OpenResty 配置通过。HTML SHA256 `a7dfb1484ad11659acfe05d7aef773b54f90105de493f9fdb0d32c05b91d3ab9`。

发布前后订单均 0，奖励 8 项、库存合计 150 及配置/库存摘要一致；币账 2084 条、合计 942472 币分一致。提醒维持 enabled=true/revision=4/原三类事件，QQ 仍关闭。未代用户兑换、创建游戏合同或发送测试消息。真实多份交付仍由用户使用确认，不把模拟测试当作真实证据。

恢复优先向前修复；一旦有多份订单，禁止回退到只退一份库存或不支持展开总量的旧程序，也禁止 Down/覆盖旧备份。旧单默认一份，原幂等键和账本保留。源码目录 `/root/glorynavy-exchange-quantity-release-20261019`，本轮操作证据在忽略目录 `.local/exchange-quantity/`。
