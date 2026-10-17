# 提醒与游戏内邮件

首批仅接入待审批提醒：福利申请与取消、兑换取消、军团贷款，以审批中心实时来源返回的可操作事项为准，排除本人申请。页面 `/reminders` 位于系统管理，仅当前站点管理员可配置与读取发送记录。QQ 保持暂停，不读取 QQ 绑定，也不恢复旧机器人。

## 配置

在 `MODULES` 中启用 `reminder`，同时启用 `identity,access,eve,approval`；Goose 93 默认关闭发送，发件角色为空。管理员在 EVE 开发者应用允许的权限中加入 `esi-mail.send_mail.v1`，然后在提醒页面选择**本人已绑定角色**，点击“授权发信”，完成同一角色的 SSO 后返回页面，勾选“待审批提醒”并保存。

普通登录仍使用既有 57 项 scopes（fittings 启用时 58 项）。只有显式 `POST /api/v1/eve/characters/{id}/reauthorize?purpose=reminder-mail` 请求在基线上增加发信权限；额外权限由服务器固定，浏览器不能自选 scopes。开始与回调都核对当前管理员、会话、绑定和指定角色，flow 保留 scopes 快照并校验实际授权。此授权不自动开启发送，之后普通重新授权可能移除发信 scope，需要再次授权发信。另一管理员可暂停原发件角色；启用或更换时须选择自己已授权的角色。

## 发送与权限

- 每分钟扫描，按账号使用独立 River 作业，载荷仅账号 ID。发送给收件账号的当前主角色，一封邮件只包含个人待办数量与 `/approvals` 链接；不含申请人、金额或业务明细，不发送给整军团、联盟或邮件列表。
- 宿主注入 `approval.PendingNotificationKeys`、当前主角色、有效绑定、管理员校验与 EVE 发信网关。提醒私有 store 仅维护配置、去重回执与发送历史，不复制审批状态，不访问来源私有 SQL。
- 发件角色必须持续属于配置账号，该账号仍是站点管理员，授权的 owner 与 generation 必须有效。发送前再次核对配置版本、主角色、待办与当前来源权限；来源读取失败不当作无待办清空回执。
- 复用 ESI 共享刷新、授权代次、限流与 POST 非缓存通道。`approved_cost=0`，不授权 CSPA 费用；收费或上游拒绝会失败，不自动花费 ISK。
- 每账号最多每 5 分钟尝试一封。当前待审阶段首次启用也会提醒，持续不变不会重复；已观察到阶段结束、随后重新进入时可再次提醒。设置修改、换发件角色、进程重启不会清空去重。
- 发送前先持久化占用；网络超时、无有效邮件 ID、5xx 或进程中断按 `unknown`，不自动重发。5 分钟后仍为 `sending` 转 `unknown`。明确拒绝等 `failed` 可由管理员点击重试，重新核对当前事实并遵守频控。`sent` 表示 ESI 返回邮件 ID，客户端收到仍需真实验收。
- 配置变更与远端发信无法构成原子事务；发出请求前完成最终检查，已提交到 ESI 的请求无法撤回。保存停用不撤销已在途邮件。

## 数据、合并与恢复

Goose 93：`reminder_settings`、`reminder_audit`、`reminder_receipts`、`reminder_deliveries`。活动回执持续保留；非活动回执与非发送中历史默认保留 90 天，配置/重试审计保留。独立 `reminder` 队列并发 1；模块关闭时不新增周期任务，已入队作业休眠，不访问远端。

合并参与方无论模块是否启用都注册：历史归到目标账号并保留首次 `original_account_id`，去重回执保守合并；来源账号作为发件人时暂停配置，不转移管理员权限或发送同意。Goose Down 在已有审计/发送记录或配置启用时拒绝。回退不支持本模块的程序前停用发送、取消 `reminder.scan.v1` / `reminder.send.v1` 未结束任务、从 `MODULES` 移除 reminder，并保留 Goose 93 和回执；有合并数据时不得回退到遗漏参与方的程序。

接口见 [OpenAPI](../../api/openapi.yaml)，页面规范见 [提醒页面](../ui/reminder.md)。官方依据：[ESI OpenAPI，兼容日期 2026-08-18](https://esi.evetech.net/meta/openapi.json?compatibility_date=2026-08-18)，`POST /characters/{character_id}/mail`，scope `esi-mail.send_mail.v1`，共享 `char-social` 桶 600/15 分钟。

实现、测试与生产状态以[项目状态](../project-status.md)为准；实际发件人授权与游戏客户端收信见 REMINDER-01。
