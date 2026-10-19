# 旗舰补贴整体优化交付记录 — 2026-10-19

## 范围

新增只读领取状态与购舰合同核验预览；已有申请可直接打开，申请展示购舰角色/舰船/成交价和预计补贴。编辑合同即禁用旧预览提交，失败可重试。个人/审批中心共用紧凑详情，按提交、批准、关联合同、发放完成展示进度；M/B/T 展示大金额，精确合同付款复制保留。保持原补贴比例、资格占用、审批与合同付款边界，普通旗舰仍暂停。契约见[中文指南](../integrations/welfare.zh-CN.md) / [English](../integrations/welfare.en.md)。

## 验证

- 隔离 PostgreSQL 旗舰/福利/统一发放/合同及 locale 的 race 回归 213.846 秒，宿主福利/底座/元数据权限回归 22.955 秒，go vet 通过。覆盖只读预览不写申请/占用、历史领取、在途申请、取消后恢复、本人和军团隔离、跨账号合同占用不泄露身份，以及大金额整数精度。
- 前端类型/构建、8 项金额与协议单测通过。最终中英文 1280/375/320px 旗舰流程 30 项通过（1.4 分钟），既有补损桌面回归 12 项通过（47.7 秒）。首次发现全数字金额未缩写，改为 M/B/T；同时修正测试历史 action 字段、英文标签和缺失词条后复跑。lint 无错误，仅组件文件导出 hook 的 Fast Refresh 提示。
- 已检查[桌面详情](../ui/reviews/welfare/capital-detail-desktop.png)、[窄屏申请](../ui/reviews/welfare/capital-preview-narrow.png)和[窄屏发放](../ui/reviews/welfare/capital-payout-narrow.png)。截图为模拟数据；未进行生产测试审批、付款或通知发送。
- GitNexus 编辑前展示入口 LOW；宿主路由注册的调用边不完整，人工补查。最终暂存/main 比较 MEDIUM（20 文件含 3 张截图、4 条已索引展示流程）。索引落后 7 次提交，新增只读服务及审批中心组件使用另行人工检查，不把索引空边视为无影响。

## 发布与线上核对

前后端 `v0.1.0-capital-flow-20261019` 来自干净且推送的 main 提交 `ad59f6021f4b1c1eb1009398227beb21ce633bde`。Goose 101 不变，无新迁移/环境配置。142 个前端文件与最终测试产物字节一致，4 个后端/测试文件与隔离测试和发布源码按换行归一匹配。源码 `/root/glorynavy-capital-flow-release-20261019`；忽略目录 `.local/capital-flow/` 保留本地构建、测试及部署材料。临时数据库和本地预览已停止。

备份 `/var/backups/glorynavy/before-v0.1.0-capital-flow-20261019-20261019T063932Z.dump`，261242402 bytes，同前缀环境与元数据备份保留；本轮未演练恢复。

服务 active/running/ready 200，NRestarts=0。首页/福利/审批/损失页面 200；新 `/api/v1/welfare/capital/check` 与原业务接口匿名 401；OpenResty 配置检查与完整发布清单通过。公网与发布资源字节相同：

- HTML SHA256：`1ce83726817dd03ca343a41f6e53dbbda1f8d9e91d39d20b57c2a7468cf8f226`
- `index-DUIWHilS.js`：`480bcdef5a46b6de195d475ed3aace876ca807f143931f744663319b0341d7f6`
- `page-CMNetWsu.js`：`70d026b3eb6d39146c961942f03baee10ec9dda2d9f76d46f6b98e4eda5e77cf`

生产未配置超级航母/泰坦规则，亦无对应申请；保持未开放。solo approved 11/cancelled 3/completed 23/rejected 14，历史日额度分配 1；提醒 enabled/revision 4/原三类事件，发信授权 ready，发送与回执各 1；QQ 关闭/绑定 0，三类专员均 0 名成员，发布前后保持。新界面真实成员购舰/发放闭环仍需业务使用验证，既有 FUL-01 待办不因模拟测试关闭。

## 恢复边界

本次无数据库行为变更，必要时前后端成对回到 `v0.1.0-loss-age-20261019`，只移除本轮只读预览与展示改进，保留 Goose 101 与后续业务数据。不要用发布备份覆盖新业务；更早版本仍受补损时限/额度/附件等既有约束。优先向前修复，部署步骤见[部署文档](../deployment.md)。


## 追加：购舰合同搜索选择

申请改用本人本军团角色的已完成购买合同，角色切换、ID/标题/物品/交易方服务端搜索、分页和清空均复用现有受保护 API；只提交原 contract_id。选择变化清除旧预览，服务端资格/船型/占用和事务复核继续生效。无后端、权限或数据库改动，可单独发布前端。

类型/构建、6 项合同协议及旗舰金额单测通过；中英文桌面 14、手机 28 项流程及 6 项加载/长名称/输入法流程通过。截图复核发现限高列表缩小隐式行高、金额越出行边界；补上按内容确定行高并增加几何边界检查后，18 项受影响流程复跑通过。已复核[桌面](../ui/reviews/welfare/capital-picker-desktop.png)及[320px 英文](../ui/reviews/welfare/capital-picker-narrow.png)。lint 无错误，原 capital hook 的 Fast Refresh 提示保留。没有生产测试申请、审批、付款或消息发送。

GitNexus 对 Apply 的上游影响为 LOW：直接调用 Board 与损失 Records；只修改旗舰分支，其他入口保持。索引落后 9 次提交，CapitalPreview 和测试 setup 无索引结果，已人工核对调用和新增独立组件。提交前 staged/main 比较均为 LOW（14 文件含 2 张截图，12 个已索引变更）；无已索引受影响流程，索引空边已按前述实际调用补查。


追加前端已从干净且推送的 main `20c7e0a337f6598a279e1c0727be8e6275ba612a` 发布 `v0.1.0-capital-contract-picker-20261019`，143 个文件与最终测试构建字节一致。后端保持 capital-flow、Goose 101，没有重启或迁移。页面 200、匿名福利/审批业务 401、完整发布清单和 OpenResty 检查正常；后端 active/running/ready 200、NRestarts=0。公开 HTML SHA256 `4602086a9dfd554cea779d0d06e9746ab80b729c68c11a42c05f54ff80922ca8`；`index-BrTi1ZTw.js` 为 `d0bf5a96e4865fd0a5a8a46e8420243ebbb90f050246d404b3d5f1d58b0fa534`，`page-teUuVAf2.js` 为 `5369cfbc957542b5694eec4ac6ba51985ac5b40c234341c7035cdc9c071fd416`，均与发布包一致。仅前端回退到 capital-flow 即恢复手填 ID，保留后端与数据。构建/验证材料在忽略目录 `.local/capital-contract-picker/`。


## 追加：稀疏分页、军团卖方与无效原因

生产只读排查确认反馈合同已完成、明细已同步，但仅含四件重型导弹发射器，没有舰船，因而不符合旗舰补贴。该角色 72 份已完成物品交换合同中有 8 份个人购买候选，旧前端误排除 3 份军团出售的合同，且原始分页后筛选导致每批仅显示少量结果。这些候选仍不等于可领取旗舰补贴。未执行真实申请、审批、支付或发送。

候选批量扫描最多 5 个原始分页，达到至少 25 个候选或读完即停止；继续加载保留原始游标，后续页失败可整批重试。`for_corporation` 描述卖方代表其军团，不能用于排除本人购买。预览以可选 reason 区分没有舰船、舰船类别不符和明细未同步，实际命令沿用 ErrCapitalPurchase 错误身份及原规则。无数据库迁移。

验证：类型/构建、10 项单测、48 项中英文 1280/375/320px 浏览器流程通过，覆盖稀疏分页自动补齐、边界继续加载、异常游标/分页失败、军团卖方、搜索/角色重置、错误提示与禁用提交。隔离 PostgreSQL 福利/统一交付/词表回归（race，226 秒）及宿主福利/权限回归（22 秒）通过；所有新增无效合同在真实 Execute 中仍被拒绝，纯预览不写申请。受影响 oxlint 无错误，仅既有 Capital hook 的 Fast Refresh 提示；曾误用不存在的 ESLint 入口，随后使用项目 oxlint 检查。

已检查桌面与 320px 英文截图，长列表保持滚动与固定提交区：[桌面](../ui/reviews/welfare/capital-contract-fix-desktop.png)、[窄屏](../ui/reviews/welfare/capital-contract-fix-narrow.png)。截图使用模拟数据。GitNexus 预编辑影响为 LOW，capitalPurchase 的直接调用为 findCapitalPurchase/guardCapitalPurchase；索引落后 11 次提交，新增 CapitalCheck/前端符号未命中，补查调用链。go vet 通过；GitNexus 暂存与 main 比较均 LOW（19 文件、13 个已索引变更，无已索引受影响流程），新增符号及共享 Execute/HTTP 错误映射已人工补查。前后端已从干净且推送的 main `0ab8da3026d5c6a864b7e59e64b3c7b8706f552c` 发布 `v0.1.0-capital-contract-fix-20261019`，Goose 101 不变。143 个前端文件及 3 个后端/测试文件与测试产物/源码一致；服务 active/running/ready、NRestarts=0，页面 200、匿名业务 401、完整清单与公开 HTML/主包/福利脚本字节校验通过，OpenResty 正常。

当前超级航母/泰坦均 enabled=true、1000 bps，仍无旗舰申请；solo 状态 approved 11/cancelled 3/completed 23/rejected 14，提醒 enabled/revision 4/原三类事件、QQ 关闭/绑定 0，发布前后保持。备份 `/var/backups/glorynavy/before-v0.1.0-capital-contract-fix-20261019-20261019T074851Z.dump`（261477459 bytes）。

必要时后端回到 `v0.1.0-capital-flow-20261019`、前端回到 `v0.1.0-capital-contract-picker-20261019`；会恢复旧分页和通用提示，保留 Goose 101 与全部业务数据。不要用备份覆盖后续业务，优先向前修复。


本轮公开 HTML SHA256 `49b0b459258c7914d9b639e9ea584c607336e633fdc6c460ca4f1733563728dd`；相关静态资源验证：

- `index-DSXxd1IF.js`：`95825459580c83085ec2724c6af8c49fbb84394701325886ae874a6275dfa561`
- `page-0L0Wbr77.js`：`ae83a675e7a2882a3307ef4037267ca090e008803684a3c07097b9bdae0ca257`

验证材料位于忽略目录 `.local/capital-contract-fix/`；没有真实申请/审批或发送测试消息，FUL-01 仍按业务使用验收。


## 追加：审批购舰凭证明细

来源详情已含完整购舰合同，旧页面凭证区只渲染编号和时间。新增展示保存的合同标题/状态、买卖双方、价格、完整年份的创建/完成/核验时间、全部物品及数量；审批默认展开，个人详情按需展开。物品可键盘聚焦滚动，长名称换行；缺名回退 ID、缺历史快照或物品待同步分别提示。没有新 API、权限、后台改动或迁移，不读取审核者无权访问的成员个人合同。

线上只读确认当前一份待审超级航母申请已保存 finished 合同、items_ready=true、4 项物品，卖方名称未缓存。此轮展示修复直接使用既有证据，不要求重提；缺失卖方名称仍以角色/军团 ID 表示，不编造名称。生产没有测试申请、审批、付款或发送。

类型/构建、11 项合同协议/金额/词表单测及受影响 oxlint 通过（仅既有 Capital hook Fast Refresh 提示）。首轮 60 项浏览器流程中 54 项通过、6 项因购舰凭证时间省略年份失败，改用已有含年份的合同日期格式并补上物品列表键盘焦点；最终 30 项中英文 1280/375/320px 受影响流程通过。已复核[桌面](../ui/reviews/welfare/capital-purchase-evidence-desktop.png)及[320px](../ui/reviews/welfare/capital-purchase-evidence-narrow.png)截图（模拟数据）。纯前端变更，不重复运行后端业务测试。GitNexus 查询定位到 ApprovalDetail/presentCases，较新 CapitalCaseContent/CapitalPurchaseEvidence 未收录；已手动核对 CaseView 到审批/个人详情的共享调用，以及读取已有 detail.purchase 的边界。GitNexus 暂存/main 比较均 LOW（15 文件、12 个已索引变更、无已索引受影响流程），索引空边已由上述源码调用检查补足。发布结果待补充。
