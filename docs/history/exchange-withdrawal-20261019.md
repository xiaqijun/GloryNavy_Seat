# 兑换直接撤销（2026-10-19）

需求：未审核、未开始办理的兑换应该直接撤销退款。之前所有 pending 都只能提出取消申请。

实现：本人 withdrawn 命令 → cancelled，整单退款、同内容版本返还全部库存、审计和状态同事务；旧取消申请可重新判断，无自动历史退款。Goose 103 保存开始发放，单笔 POST 与合并批次共享订单锁。合同证据/读取错误/已开始保留原审核；禁止自审和代撤销。

GitNexus：DecideOrder、RewardOrders、Handoff、CreateSettlementBatch、宿主 New 及 UI 主要调用 LOW；ReadDelivery 的四个直接调用（列表、审批详情、合同核验、取消保护）为 HIGH，已告知并列入回归。索引落后 21 个提交，已用源码复核新批次与 UI 路径；未改共享错误响应函数。

TypeScript/Vite、受影响 lint、OpenAPI 解析通过。新增浏览器流程在 1440/375/320px 共 12 项通过：pending 和历史取消待审直接撤销、5 份整单退款、负余额刷新、已开始仍审核、GET 不开始及显式 POST 后显示合同复制；既有取消/发放/欠额回归 9 项通过。已检查[桌面](../ui/reviews/exchange-withdrawal/desktop.png)与[320px](../ui/reviews/exchange-withdrawal/narrow.png)确认弹窗，无横向溢出。

隔离 PostgreSQL 18.6 全 exchange race 422.610s、locale 1.076s，福利/结算相关 race 107.398s 与 store 1.072s 通过；覆盖本人/他人、旧申请、多份、重复提交、发放标记、合同与缺失证据、读取失败、内容换版、退款回滚、单笔及批次开始竞争，以及合并批次所有标记同事务/失败回滚/重放。宿主相关 race 22.906s 与 go vet 通过；上线证据如下。测试使用隔离库/本地浏览器夹具，没有代审或修改生产币账。


提交前 GitNexus staged/compare-main 共检测 29 个文件、55 个变更符号、40 个影响项，总评级 critical，已告知用户。范围为兑换退款/交付读取、福利批次、宿主注入和对应 UI，没有扩展为其他币账来源；结合过期索引进行了源码复核及上述回归。

前后端于 2026-10-19 从干净、已推送 main `b8ff6e7cfe701659841a1256378e64655d163a2c` 发布 `v0.1.0-exchange-withdraw-20261019`。143 个前端文件逐字节匹配测试构建；734 个后端文件在隔离测试目录和发布源码中按统一换行 SHA-256 一致。Goose 103、River 迁移成功，服务 active/running、NRestarts=0、ready 200；测试容器已清理。

备份 `/var/backups/glorynavy/before-v0.1.0-exchange-withdraw-20261019-20261019T104314Z.dump`，262078671 bytes；同批环境备份按运维流程保存。发布前后订单共 2 笔（pending/cancel_requested 各 1），订单哈希 `a64ee1e380f8c11c40a9206ccc7176dd`；8 项库存共 124，哈希 `7f298d6242bfca31ed5d7f028ffe6e69`；币账 2088 条、合计 682472 币分均一致。提醒配置未改、OneBot 仍停用；没有代替用户撤销或审批订单，没有生产测试退款。

公开首页、兑换、奖励和审批页面均 200；匿名 exchange context/rewards/orders/handoff 及 approval context 均 401，OpenResty 配置检查通过。完整发布文件清单校验及公开 HTML、主包、兑换包字节一致：HTML `b44f477a95469cb8c34b00052c4c046fca4778603ba1457dc42b605f7a06bbdd`，主包 index-DCAwutNI.js `39c346f88178a7e617c316be6a25433365565d8c34fda38d7520001aea75c641`，兑换包 rewards-panel-Bc3QVUAB.js `786680b11dc57dcc551ec3b9e4a659650b724e6208fb90c8fb67c12ce98c9abc`。真实游戏交付验收继续按 backlog FUL-01。
