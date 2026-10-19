# 兑换直接撤销（2026-10-19）

需求：未审核、未开始办理的兑换应该直接撤销退款。之前所有 pending 都只能提出取消申请。

实现：本人 withdrawn 命令 → cancelled，整单退款、同内容版本返还全部库存、审计和状态同事务；旧取消申请可重新判断，无自动历史退款。Goose 103 保存开始发放，单笔 POST 与合并批次共享订单锁。合同证据/读取错误/已开始保留原审核；禁止自审和代撤销。

GitNexus：DecideOrder、RewardOrders、Handoff、CreateSettlementBatch、宿主 New 及 UI 主要调用 LOW；ReadDelivery 的四个直接调用（列表、审批详情、合同核验、取消保护）为 HIGH，已告知并列入回归。索引落后 21 个提交，已用源码复核新批次与 UI 路径；未改共享错误响应函数。

TypeScript/Vite、受影响 lint、OpenAPI 解析通过。新增浏览器流程在 1440/375/320px 共 12 项通过：pending 和历史取消待审直接撤销、5 份整单退款、负余额刷新、已开始仍审核、GET 不开始及显式 POST 后显示合同复制；既有取消/发放/欠额回归 9 项通过。已检查[桌面](../ui/reviews/exchange-withdrawal/desktop.png)与[320px](../ui/reviews/exchange-withdrawal/narrow.png)确认弹窗，无横向溢出。

隔离 PostgreSQL 18.6 全 exchange race 422.610s、locale 1.076s，福利/结算相关 race 107.398s 与 store 1.072s 通过；覆盖本人/他人、旧申请、多份、重复提交、发放标记、合同与缺失证据、读取失败、内容换版、退款回滚、单笔及批次开始竞争，以及合并批次所有标记同事务/失败回滚/重放。宿主相关 race 22.906s 与 go vet 通过；上线证据待补充。测试使用隔离库/本地浏览器夹具，没有代审或修改生产币账。
