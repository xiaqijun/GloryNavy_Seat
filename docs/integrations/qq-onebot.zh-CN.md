# NapCatQQ 接入

**2026-10-28 方向已替换：**用户要求切回[官方 QQ 机器人](qq-official.zh-CN.md)。下文为历史接入/恢复证据，不是当前操作步骤；新程序的 OneBot URL 返回 410，旧配置/绑定/发送回执保留，不自动转成官方身份。本轮发布状态见[项目状态](../project-status.md)。

更新：2026-10-17。[English](qq-onebot.en.md)。用户已要求切换第三方机器人，覆盖原“仅官方 Bot API”限制。当前发布与真实联调以[项目状态](../project-status.md)为准。

## 选型与范围

选择 [NapCatQQ](https://github.com/NapNeko/NapCatQQ)，2026-10-17 GitHub API 查询为 11,007 Star；作为未修改的独立服务运行，本站通过 OneBot 11 HTTP 接口接入。正式 Docker 镜像 v4.18.37 固定摘要见[部署示例](../../deploy/compose.napcat.example.yaml)；调查时源码最新 v4.18.38 尚无对应正式镜像标签。保留项目自身许可，未复制 NapCat 源码到本站；其许可含非商业使用限制，详见[第三方声明](../third-party-notices.md)。

使用普通 QQ 账号扫码登录，不使用旧官方 AppID、App Secret 或 OpenID。此次实现群内验证码绑定、真实群列表选择及管理员“测试 @我”。待审批提醒已实现，发布与真实验收见项目状态；其他业务通知和自动同意入群尚未实现。

## 使用

1. 运维完成私有 NapCat 部署，用户扫码并将该 QQ 拉进目标群。
2. 管理员在 `/community` 保存机器人 QQ 号、API 地址、访问令牌、回调密钥并启用。两种密钥至少 32 个非空白字符，均只写、加密存储；留空保留原值。并发保存按 revision 拒绝覆盖。
3. 点击“连接并获取群”，核对登录身份和在线状态，勾选接收群并保存。群选择基于实际加入的群，也可移除已保存但当前未返回的群。
4. `/account` 填写真实 QQ 号、生成验证码，用同一个 QQ 在选定群中直接发送完整验证码。无需 @ 或指令前缀；10 分钟有效，一次性使用。
5. 管理员绑定完成后，在目标群点击“测试 @我”。发送目标只能是当前管理员已验证的 QQ，消息为固定测试文案；HTTP 成功不替代客户端 @ 显示验收。

第一次保存 OneBot 配置即切换传输方式，停用开关只停用 NapCat，不会恢复旧官方回调。旧 OpenID 绑定保留为历史，但不再满足当前 QQ 确认；必须重新通过群内验证码确认。QQ 号或版本变更后旧验证失效。账号合并删除源账号 OneBot 绑定，既不复制社区资料也不把数字 QQ 映射成官方 OpenID。

## 接口和校验

`GET/PUT /api/v1/community/qq/onebot/settings`、`GET /status`、`POST /test`（后两项同前缀）使用 `community.group.manage`，宿主每次检查当前站点管理员；写操作保留 Origin、会话、CSRF。密钥不在 API 响应、错误或日志中回显。API 地址限 HTTPS 或 loopback HTTP，客户端禁用重定向、8 秒超时，发送不自动重试，避免不确定结果重复提醒。

公共 `POST /api/v1/community/qq/onebot/webhook` 校验原始请求体 `HMAC-SHA1`（NapCat HTTP client 的 token）和 `X-Signature: sha1=<hex>`，同时校验配置的 self_id 和时间窗（过去 5 分钟、未来 1 分钟）。64 KiB 上限。只处理 normal 群消息中完整文本验证码；普通聊天、自身消息、私聊、匿名消息、图片/引用/@ 等混合段以及未配置群静默忽略。发送者数字 QQ 必须等于本站保存值。

事务共享锁复核配置版本/群范围，通过宿主注入 identity.LockActiveAccounts 防止合并后迟到写入；按事件及 QQ 串行、锁定验证码与资料版本，原子写入绑定、确认、码消费和事件哈希。重复事件不再回复，冲突载荷拒绝。消息正文不落库，仅对验证码尝试保存事件标识和 SHA-256 哈希。回复采用 OneBot quick operation 的 `at` + `text` 消息段；无通用聊天回复。数据库提交后若网络丢失，绑定仍成立，网站状态可确认；不会为补回复重复绑定。

## 运维与升级

Goose 91 新增模块私有配置、群和数字 QQ 绑定表，原验证码及审计表继续使用。加密复用 EVE_TOKEN_KEY 派生的 QQ 配置密钥，两个凭据分别保存。存在 OneBot 配置或绑定时 Down 拒绝；不能回退为忽略新身份类型的旧程序。需要恢复时停止 NapCat 回调，核对数据库/配置备份和旧确认状态后按恢复指南执行。

NapCat 与应用分别部署，示例不含任何真实凭据。`qq/`、`config/`、`plugins/` 为持久目录，UID/GID 1000 可写；父目录仅运维可读。预建 `config/webui.json`，设置随机 token、host `0.0.0.0`、port `6099`。预建 `config/onebot11.json` 的 network：

- httpServers：enable=true、host=`0.0.0.0`、port=3000、token=`<ONEBOT_ACCESS_TOKEN>`、messagePostFormat=`array`、enableCors=false、enableWebsocket=false。
- httpClients：enable=true、url=`https://<SITE_HOST>/api/v1/community/qq/onebot/webhook`、token=`<WEBHOOK_SECRET>`、messagePostFormat=`array`、reportSelfMessage=false。
- websocketServers、websocketClients、httpSseServers 均为空数组；debug=false。

容器端口仅映射宿主 127.0.0.1；WebUI 经 SSH 隧道访问，不发布到公网。无需 privileged、宿主网络或 Docker socket。Compose 限制日志为 3×10 MiB、关闭 core dump。检查 `get_login_info`、`get_status`、`get_group_list` 后才启用网站配置；不要把 QQ 登录缓存、二维码、WebUI token 或截图写入仓库。备份持久目录并沿用私人备份权限，停用旧官方配置不删除其历史审计。

参考：[NapCat 开发接口](https://napcat.napneko.icu/develop/)、[消息段](https://napneko.github.io/develop/msg)、[Docker 项目](https://github.com/NapNeko/NapCat-Docker)。

## 待审批 @ 提醒（Goose 92）

在 `/community` 的每个接收群勾选“审批提醒”并保存；新群和升级时默认关闭。需要同时启用 community、approval 及其依赖模块。首次开启会提醒当前仍待处理的事项。福利申请/取消审核、奖励兑换取消审核、军团贷款审批沿用来源模块的实时权限；站点管理员和有对应军团审批权限的人员均可接收，但必须完成当前 QQ 版本的 OneBot 验证，且在该接收群内。自己的申请、无权处理、已处理、待发放及补资料项不提醒。个人出借不在审批中心范围。

宿主注入 approval.PendingNotificationKeys，直接读取来源授权与待审批页，不使用索引或五秒权限缓存；失败/不完整结果不关闭回执。community 不读取其他模块私有 store，不保存申请金额、姓名、原因、审批状态或付款记录，只保存不透明引用及通知回执。消息按成员使用 OneBot `at` 和 `text` 段，显示“有 N 项待审批”与站点 `/approvals` 链接，不含具体申请内容。

共享 River 的独立 community 队列每分钟扫描（启动时扫描一次），任务名为 `community.approval-scan.v1` 与 `community.approval-group.v1`。每群最多每分钟一条、每条最多 20 位审批人，其余未领取记录留待后续扫描；同一连续待审批阶段只提醒一次，新增事项或取消阶段会再提醒。只有完整扫描观察到离开并重新进入待审批时，才重新提醒同一阶段；一分钟内往返的状态不保证再次提醒。关闭再开启或保存群配置不重置去重回执。

发送前复核机器人登录/在线、实际群成员、实时权限/状态、配置 revision、有效绑定及账号未合并；从群移除、撤权或改号后跳过。离线、权限来源暂时失败等发送前错误可由后续扫描重试。领取回执先于网络发送持久提交，群级锁串行处理并发；OneBot 无发送幂等键，超时、未返回消息编号或提交后进程退出均不自动重发。该策略可能漏掉一次提醒，但避免重复 @；审批中心始终是待办依据。发送状态为 sending/sent/unknown，遗留 sending 五分钟后转 unknown；sent 仅表示返回消息编号，不表示已读或客户端正确显示。

Goose 92 的 community_approval_notices 与 community_approval_deliveries 为模块私有。已失效回执及发送批次保留 90 天，持续待办回执保留以防重复；不清理审批或业务审计。合并账号不转移提醒目标，取消源账号活动回执、保留发送批次，目标按自己的权限与 QQ 绑定重新判断。回退前关闭提醒并取消两种未结束 River 任务；已启用或已有发送记录时 Down 拒绝，保留回执，不能回退到不认识这些任务的程序直接接单。

成员查询兼容：v4.18.37 的[列表实现](https://github.com/NapNeko/NapCatQQ/blob/v4.18.37/packages/napcat-onebot/action/group/GetGroupMemberList.ts)会异步刷新缓存；实查曾出现仍在群内但列表返回空。候选查询使用缓存，空列表作为不可用；发送前对目标调用 get_group_member_info（no_cache=true），核对 group_id/user_id，失败不领取发送回执。机器人重启后可能需要重新扫码；确认登录后在受保护 WebUI 设置该账号自动登录，仍以真实在线状态为准。
