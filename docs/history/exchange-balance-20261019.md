# 兑换负余额 — 2026-10-19

## 后续：移除额外欠额提示

用户要求不再显示余额卡下方的“果壳币欠额……补足前暂不可兑换”文字，保留带符号余额和余额不足时的禁兑。只删除独立提示，未改后台或余额计算。RewardsPanel 影响分析 LOW，直接调用为 Workspace，间接为 ExchangePage；索引落后 23 个提交，已对照源码。既有欠额布局回归移除过时的提示断言，继续检查负余额、禁兑和三尺寸布局。TypeScript、Vite 构建及受影响 lint 通过；既有欠额响应式流程通过，验证负数余额、兑换禁用及 1440/375/320px 无横向溢出。已检查[桌面](../ui/reviews/exchange-debt-note/1440.png)与[375px](../ui/reviews/exchange-debt-note/375.png)截图，额外提示已移除。上线证据待补充。

## 前次负余额展示交付

只读事务核对订单状态、占用/退款流水、来源入币及预警消费。反馈单为 `cancel_requested`，原币额继续占用；对应测试入币已冲正，所以可用余额为负。ShopBalance 计算与账本净额相同，未发现漏记退款，不修改生产业务记录。取消需要非本人管理员核验未交付并批准，批准后整单退回。

前端 RewardsPanel 原先对 available_minor 使用 Math.max(0, …)，导致欠额在余额卡显示 0。改为带符号金额并以错误色强调，保留欠额提示和禁止兑换。API、币账、库存、取消规则、后端版本及 Goose 102 均不变。

GitNexus upstream LOW：RewardsPanel 直接影响 Workspace，间接 ExchangePage；索引落后 19 个提交，已补充源码核对。测试回调未被索引，按现有欠额布局测试定位。

TypeScript、Vite 构建、受影响 lint 通过。既有“成员兑换及申请取消保持余额”和“欠额响应式布局”两个浏览器流程通过，新增断言验证余额卡显示 -5，兑换仍禁用；1440/375/320px 无横向溢出，截图已检查：[桌面](../ui/reviews/exchange/rewards-desktop-1440.png)、[375px](../ui/reviews/exchange/rewards-desktop-375.png)、[320px](../ui/reviews/exchange/rewards-desktop-320.png)。生产只读诊断，浏览器测试使用本地夹具，未审批本人订单或改写真实币账。


## 发布核验

前端 `v0.1.0-exchange-balance-20261019` 已从干净且推送的 main `ab7c86b9a5084d2544df24a9330016859783d680` 发布。143 个文件与测试构建一致，上传/激活后 SHA256 清单通过。首页、兑换、奖励库、审批页 200；匿名兑换上下文/奖励/订单/合同信息与审批上下文均 401。OpenResty 正常，公开 HTML、主包和兑换包与服务器发布产物字节一致。

- HTML SHA256：`46fe13760042196a3365e0d2c24e94d5f23ef24ca3ca22d5bc65d3d450f8e4d2`
- 主包：`index-8RhFYzVS.js`，SHA256 `178ee7008480916fc0646e1cc734cf28785897809bbbc18bda677b29f90c84ea`
- 兑换包：`rewards-panel-Dsh0K8QP.js`，SHA256 `9ee9269cdb18cb38f06eb15859e9317c0fc6303a9c7c13b81717180f808f2fdb`

提交前 GitNexus staged/compare 为 MEDIUM（10 个文件、5 个符号、3 条 RewardsPanel 流程），人工差异确认只含余额展示、样式、原测试增强、文档和截图。发布前再次只读核对，反馈单仍取消待审核，计算余额与账本净额一致。未执行后端发布、迁移、重启或真实兑换/退款操作。后端保持 exchange-quantity、Goose 102；必要时只回退前端至上一版 exchange-records。
