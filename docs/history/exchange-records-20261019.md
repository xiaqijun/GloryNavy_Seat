# 兑换记录排版 — 2026-10-19

## 范围

仅调整兑换中心的个人/兼容管理记录：分栏摘要、明确状态、集中操作、折叠完整合同及奖励明细；M/B/T 缩写保留精确数值入口。审批中心继续使用原详情，业务请求、扣币、库存、取消规则及权限不变。无迁移，后端保持 `v0.1.0-exchange-quantity-20261019`、Goose 102。

## 验证

TypeScript、Vite 最终构建及受影响前端 lint 通过。浏览器夹具检查中文/英文 × 1280/375/320px 六种环境，包含五种订单/交付状态、长名称和历史结算编号、金额缩写与精确值、键盘展开、取消/合同弹窗及管理员/普通成员/审批中心入口，均无横向溢出或页面异常。截图：[桌面](../ui/reviews/exchange/records-desktop.png)、[320px 英文详情](../ui/reviews/exchange/records-narrow.png)。

浏览器使用本地夹具，不代表生产订单，未提交真实兑换或发送通知。最终兑换/取消/退款/焦点回归 9 项、多份兑换 6 项均通过。首轮并行时两项较长管理流程超过默认 30 秒总预算，单 worker、120 秒预算复核均通过，没有修改业务代码规避。线上核验见下。

GitNexus `RewardsPanel` upstream 为 LOW：直接调用者 Workspace，间接 ExchangePage，无已索引执行流程扩大。索引落后 17 个提交，已补充源码调用检查；新增 OrderRecord 仅由 RewardsPanel 使用，原共享 DeliveryInfo 未修改。测试回调不在索引中，按原测试文件核对断言调整范围。

暂存及相对 main 的 detect_changes 均为 MEDIUM（12 个文件、6 个符号、4 条流程）；人工差异确认仅记录视图、新展示组件、样式、词条、测试定位与文档/截图。索引将行号移动映射到 coin/审批流程，但 coin 与共享 DeliveryInfo 均未改变。

## 发布

前端 `v0.1.0-exchange-records-20261019`，来源干净且已推送的 main `98b1c168237913bf77d3a2a34f2bffe9fd2d8a4e`。143 个文件与最终测试产物逐一匹配，上传后和激活后完整 SHA256 清单均通过。公开 `/`、`/exchange`、`/rewards`、`/approvals` 为 200；匿名兑换上下文、奖励、订单、合同信息及审批上下文均为 401。公开 HTML、主包和兑换包与发布文件字节相同，OpenResty 检查通过。

- HTML SHA256：`6d24548895351bd51d888f66f5d1ca8aa0c9861924a5d3f0c302b4eabed5b3db`
- 主包：`index-UB_ehxbb.js`，SHA256 `48e9c98f615cc41d583779e33de801f10dedbb1c93df530851eaff8ba6b4caac`
- 兑换包：`rewards-panel-LxxDr4SD.js`，SHA256 `04205cc756ef80dade178c898aedf0a2e77ccf004ea70bcf396f6e9174b33275`

后端保持 `v0.1.0-exchange-quantity-20261019`、源提交 `773fd63`、Goose 102；只读核验 active/running、ready 200、NRestarts=0。未执行后端发布、迁移、重启、真实兑换或通知发送。前端必要时可按既有边缘发布脚本回退到上一版 exchange-quantity；无需回退后端或数据库。
