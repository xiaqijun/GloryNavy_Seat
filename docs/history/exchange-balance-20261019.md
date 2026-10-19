# 兑换负余额 — 2026-10-19

只读事务核对订单状态、占用/退款流水、来源入币及预警消费。反馈单为 `cancel_requested`，原币额继续占用；对应测试入币已冲正，所以可用余额为负。ShopBalance 计算与账本净额相同，未发现漏记退款，不修改生产业务记录。取消需要非本人管理员核验未交付并批准，批准后整单退回。

前端 RewardsPanel 原先对 available_minor 使用 Math.max(0, …)，导致欠额在余额卡显示 0。改为带符号金额并以错误色强调，保留欠额提示和禁止兑换。API、币账、库存、取消规则、后端版本及 Goose 102 均不变。

GitNexus upstream LOW：RewardsPanel 直接影响 Workspace，间接 ExchangePage；索引落后 19 个提交，已补充源码核对。测试回调未被索引，按现有欠额布局测试定位。

TypeScript、Vite 构建、受影响 lint 通过。既有“成员兑换及申请取消保持余额”和“欠额响应式布局”两个浏览器流程通过，新增断言验证余额卡显示 -5，兑换仍禁用；1440/375/320px 无横向溢出，截图已检查：[桌面](../ui/reviews/exchange/rewards-desktop-1440.png)、[375px](../ui/reviews/exchange/rewards-desktop-375.png)、[320px](../ui/reviews/exchange/rewards-desktop-320.png)。生产只读诊断，浏览器测试使用本地夹具，未审批本人订单或改写真实币账。发布证据待补充。
