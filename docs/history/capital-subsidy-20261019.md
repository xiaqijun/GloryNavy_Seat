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
