# QQ 与 KOOK 社区资料

> 状态索引（2026-09-23）：本指南对应的业务已上线；未交付的扩展与真实验收缺口见[待办](../backlog.md)，当前版本见[项目状态](../project-status.md)。下文带日期/Goose 的发布前记录保留当时语境，不能据此认定当前未发布；升级执行仓库最新迁移，不停在模块初始版本。

更新：2026-09-29。[English](community-profile.en.md)。手填资料、编辑、资料完整度门禁、官方 QQ 群入群审批回调、管理员补偿同步和管理员前端群配置已在本地实现；真实 QQ 开放平台联调和 KOOK 机器人仍未完成；平台 OAuth 不属于已确认需求。

## 1. 使用与状态

在 `/account` 的“社区资料”填写 QQ 号和 KOOK 昵称，保存后可通过编辑图标修改。资料属于本站用户；切换查看角色、更换主角色或使用小号登录均共用原资料。

- QQ：本站输入规则为 5–12 位 ASCII 数字，不以 0 开头，作为字符串保存。
- KOOK：去除首尾空白后 1–64 个 Unicode 字符，不含控制字符，保留内部空格及表情。
- 两项都合法才保存并标记资料完整；规则是本站校验，不表示 QQ/KOOK 已核验账号存在或昵称归属。
- 保存后显示“待入群确认／待进入确认”。同 QQ 或同昵称不触发账号合并；未确认的输入不作为唯一身份凭据，也不设置全站唯一约束。

`complete` 只表示资料齐全；每个平台另有 `unfilled`、`pending`、`confirmed` 状态。成员不能提交确认状态。修改已确认项撤销该平台旧确认，另一平台保持不变；保存相同内容不增加版本、不撤销确认。

## 2. 接口与模块边界

`community` 依赖 `identity`，拥有私有 SQL/store。`GET /api/v1/community/profile` 与 `PUT /api/v1/community/profile` 仅处理当前会话用户，不接收目标用户 ID。写操作需同源 Origin、本站会话及 `X-CSRF-Token`；请求体最大 4096 字节，拒绝额外字段。

PUT 请求字段：`qq_number`、`kook_name`、`version`；版本为十进制字符串，空资料为 `"0"`。并发修改返回 409，页面保留输入，可主动“读取最新资料”后修改重试。完整响应见 [OpenAPI](../../api/openapi.yaml)。

启用 community 后，宿主在受保护业务接口执行资料完整度门禁，不因本站管理员身份绕过。资料未齐返回 `403 profile_required`。会话、退出、角色列表/管理、EVE 添加/重授权、本人授权摘要与社区资料读写保留可用；未确认入群不会阻止已填齐资料的业务授权检查。`/` 是公开介绍页，`/system` 等业务页面沿用宿主会话门禁；运维 API 的匿名元数据边界另见待办 SEC-01。

## 3. 存储、升级与确认扩展

迁移 `00008_community_profile.sql` 新增：

- `community_profiles`：以本站用户 ID 为主键，保存两项资料、总版本及各平台字段版本。
- `community_profile_events`：保存用户、资料版本、变化的平台及时间；不复制历史 QQ/昵称到审计内容。
- `community_confirmations`：保存用户、平台、字段版本、来源、操作者、事件 ID、确认时间与失效时间。来源/事件 ID 唯一，每个用户平台最多一个未失效记录。

资料写入、版本递增、旧确认失效及变更审计在同一事务完成，并锁定用户资料行。读取确认时还要求字段版本匹配，迟到旧版本记录不能确认新资料。历史确认保留失效标记，不作为当前状态。

迁移 `00050_community_qq_bot.sql` 增加 `community_bot_events`，保存官方 QQ 回调事件的来源、编号、原始体哈希和接收时间；不保存机器人 Token 或 EVE 凭据。生产是否已执行以项目状态记录的 Goose 版本为准。

迁移 `00051_community_qq_official.sql` 增加官方 QQ 一次性绑定码和 `openid` 绑定。官方平台事件只提供 `openid`，不能把消息发送者直接当作数字 QQ 号；旧的 C2C 绑定流程仍兼容，但新入群流程不要求私聊机器人。

迁移 `00052_community_qq_group_join.sql` 增加入群申请与群范围绑定。迁移期间可用 `QQ_BOT_GROUP_OPENIDS`（逗号分隔的 bot-scoped Group OpenID，不能填 QQ 数字群号）作为兜底；管理员完成 Goose 53 后从 `/community` 维护群配置。成员在本站填写 QQ 号后生成 15 分钟有效的一次性 8 位申请码，把申请码放入 QQ 入群验证消息或审核问答。官方 `GROUP_JOIN_REQUEST` 到达后，本站只对已配置群、未过期且申请码匹配的申请调用官方审批接口；`GROUP_MEMBER_ADD` 到达后，再把群范围的 `member_openid` 与本站账号绑定，并将当前 QQ 资料标记为已确认。申请创建时冻结 QQ 资料版本，修改 QQ 号会使旧申请不能确认新资料。

迁移 `00053_community_qq_group_settings.sql` 增加管理员维护的群配置和初始化标记；迁移 `00054_community_qq_bot_settings.sql` 增加官方 QQ Bot 的数据库配置。管理员可在 `/community` 的 QQ 机器人管理页维护 AppID、API 地址、群 OpenID、显示名称和启用状态；`/account` 仅维护个人 QQ/KOOK 资料。App Secret 使用只写字段并由服务端加密保存，页面和 API 不回显。保存后立即更新回调与补偿同步使用的配置。首次保存前，`QQ_BOT_APP_ID`、`QQ_BOT_APP_SECRET`、`QQ_BOT_API_BASE` 和 `QQ_BOT_GROUP_OPENIDS` 仅作为迁移兜底；数据库保存后以界面配置为准，允许清空旧环境列表。

官方 QQ 回调地址为 `PUBLIC_ORIGIN/api/v1/community/qq/official/webhook`。服务端使用当前数据库配置获取短期 access token，并按官方 `X-Signature-Timestamp` 与 `X-Signature-Ed25519` 完成正式事件校验；首次配置回调的 `op=13` 验证握手不带这两个请求头，只返回用 `event_ts + plain_token` 生成的 Ed25519 签名。入群审批使用官方 `GET /v2/groups/{group_openid}/join_request_list` 分页读取申请，再用 `POST /v2/groups/{group_openid}/approval_join_request/{member_openid}` 携带 `op=approve` 审批；平台事件没有数字 QQ 号，群 OpenID 与成员 OpenID 均按机器人范围使用。`POST /api/v1/community/qq/group/application` 受本站会话和 CSRF 保护，`GET /api/v1/community/qq/group/applications`、`POST /api/v1/community/qq/group/sync`、Bot 配置和群配置接口仅管理员可用。回调未送达时，管理员同步接口按配置群轮询并保持幂等；平台频控、拒绝或网络失败写入申请状态，不伪造已入群。API 基地址默认 `https://api.bot.qq.com`，本地测试可显式改为 loopback 地址。接口业务错误按响应 JSON 的 `code`/`err_code` 判断，不仅依赖 HTTP 状态。数据库配置的加密根密钥复用服务端 `EVE_TOKEN_KEY` 派生，不能在前端或日志中暴露。

实现依据：[QQ 机器人 API v2 启动接入](https://bot.q.qq.com/wiki/develop/api-v2/)、[获取 access_token](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/access-token.html)、[API 调用指南](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/api-call-guide.html)、[事件订阅与通知](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/interface-framework/event-emit.html)、[安全和授权](https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/interface-framework/sign.html)、[用户申请加群事件](https://bot.q.qq.com/wiki/develop/api-v2/autogen/event/group_join_request.html)、[单聊消息事件](https://bot.q.qq.com/wiki/develop/api-v2/autogen/event/c2c_message_create.html)、[群 @ 机器人消息](https://bot.q.qq.com/wiki/develop/api-v2/autogen/event/group_at_message_create.html)。

KOOK 适配器和真实 QQ 平台联调仍待实现。官方平台不向本站发送 EVE 凭据或本站用户 ID。

启用模块以当前配置为准。已有显式 MODULES 配置须添加 community，执行 `npm run db:migrate` 后重启 API；`dev:external` 自动迁移。首次引入为 Goose 8，官方 QQ 事件为 Goose 50/51，群审批为 Goose 52，群与 Bot 前端配置为 Goose 53/54；当前统一迁移见项目状态，foundation marker 仍为 1。迁移期间可保留环境变量作为兜底，完成管理员首次保存后由 `/community` 维护；旧用户首次进入 `/account` 补填个人资料。

## 4. 验证范围

已覆盖输入校验、账号级共享、只撤销对应平台确认、无变化保存、迟到旧版本确认、并发覆盖防护、越权字段/CSRF/匿名拒绝、管理员资料门禁，以及官方 Ed25519 验签、验证握手、绑定码过期、openid 冲突、官方业务错误码、入群申请码匹配、重复事件、资料版本变更和群成员绑定。前端覆盖字段错误、保存与修改、确认提示、入群申请码生成、失败重试和版本冲突恢复；真实 QQ 平台联调仍待完成。

## 管理员读取

管理员经 `/api/v1/access/members/{user}/data` 查看 QQ／KOOK 值与确认状态，由宿主注入 community.Get 服务；不暴露编辑版本、凭据或确认写入口。原 `/community/profile` 始终指当前登录账号，读写不接受目标账号参数。见[成员页面](../ui/members.md)。
