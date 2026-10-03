# Linux 生产部署

## 预警小时价格展示与连续区间合并（2026-10-03）

已发布 `v0.1.0-sentry-continuous-billing-20261003-r2`，同时更新 Seat 后端和公网静态前端。页面统计卡显示预警消费小时价格与监控奖励小时价格；Seat 对账 worker 将同账号、密钥、客户端的相邻认证心跳合并后按秒计价，消费查询再按冻结价格合并连续记录。无新增 Goose/River 迁移；应用 ready、公网首页 200、匿名受保护接口 401 和 OpenResty `nginx -t` 已通过。

## 预警监控奖励统计修复（2026-10-03）

已发布 `v0.1.0-sentry-reward-display-20261003-r2`。消费统计读取同时兼容历史 `sentry-monitor:` 和批量结算 `sentry-monitor-batch:` 前缀，修复币账已有奖励但页面显示为 0 的问题。应用 `active/ready`、生产统计查询、公网首页 200 和 OpenResty `nginx -t` 均通过。

## 批量结算已发合同归入已处理（2026-10-03）

已发布 `v0.1.0-contract-batch-approval-history-20261003`。批量合同已同步但等待主角色接取时，后端返回 `delivery_status=awaiting_acceptance`，审批中心将其从“待发放”移到“已处理”，详情仍显示“等待领取合同”；合同未同步、内容不匹配或其他异常不改变原分类。批次 #6 生产数据仍为 `pending`、3 项待处理、完成数 0，三项错误均为“等待接收角色完成合同”。本轮无 Goose/River 新迁移，生产 Goose 66；应用 `active/ready`、`NRestarts=0`，公网前端 `current`、首页/登录 200、容器静态入口和 OpenResty `nginx -t` 检查通过。

## 预警在线时长收费改造（已发布，2026-10-03）

本轮新增 Goose 66 `sentry_client_usage_reconcile_state`，并要求同时发布 EVE Sentry
的 `seat_client_usage` 导出与 Seat 的客户端在线区间对账 worker。新 worker 只按认证心跳
在线秒数和小时价格结算；旧事件/投递/ACK worker 不再调度新收费，旧队列任务会被取消。
本轮已发布 Seat `v0.1.0-sentry-heartbeat-time-20261003-r2` 与 EVE Sentry
`d77eab7`。生产 Goose 66 已执行；应用 ready、公网首页/登录 200、OpenResty
`nginx -t` 通过。真实心跳、正式价格和 exchange 币流水仍需现场验收，部署成功不代表
真实扣币已验收。

随后发布 Seat `v0.1.0-sentry-monitor-reward-batch-20261003`，修复监控贡献以远端密钥 ID
匹配 Seat 本地密钥的问题；每段在线证据独立保存，同一对账页和账号的奖励合并为一次币账入账。
回放历史游标后生产已核到 5,908 条奖励证据、60 条合并币账记录（共 618 个最小币单位）。
应用 ready、公网首页 200、静态站点切换和 OpenResty `nginx -t` 已通过。

## 批量结算合同标题同步修复（2026-10-03）

已发布后端 `v0.1.0-contract-batch-title-sync-20261003`，公网前端使用同批次静态产物。EVE 合同标题最多保存 50
个字符，批量结算的 `BATCH-YYYYMMDD-UUID` 编号可能被游戏截断；同步查询现兼容完整编号
和确定性的前 50 个字符，并继续校验当前主角色、发放方、整数 ISK、物品集合和合同状态。
应用与公网静态站点已原子切换；生产 Goose 66 已由并行 Sentry 发布登记，本轮 River 无新增迁移。批次 #6 已从
“等待合并合同同步”更新为“等待接收角色完成合同”，证明该合同已经同步；当前仍需主角色
在游戏中接取并完成合同。应用切换前备份为
`/var/backups/glorynavy/before-v0.1.0-contract-batch-title-sync-20261003-20261003T133223Z.{dump,env}`；
应用 `active/ready`、重启次数 0、公网首页/登录和 OpenResty `nginx -t` 均通过。

## 批量结算主角色名称显示（2026-10-03）

已发布 `v0.1.0-contract-batch-recipient-name-20261003`。批次详情的合同接收人改为当前主角色名称；后端仍保留角色 ID 供合同核验，浏览器不再把 ID 当作接收人展示。批次名称改动本身无新增迁移；发布包同步执行了工作区已有的 Goose 65，River 无新增迁移。应用机与公网 1Panel 静态站点已原子切换，应用服务 active/ready、`NRestarts=0`，备份为 `/var/backups/glorynavy/before-v0.1.0-contract-batch-recipient-name-20261003-20261003T120711Z.{dump,env}`；公网前端上一版本保留为 `releases/v0.1.0-contract-batch-main-recipient-20261003`。首页/登录 200、批量结算详情匿名 401、容器静态入口和 OpenResty `nginx -t` 通过。

## 批量结算历史主角色修复（2026-10-03）

已发布 `v0.1.0-contract-batch-main-recipient-20261003`。应用机
`/opt/glorynavy/current` 与公网 1Panel 静态站点 `current` 已原子切换；本轮无
Goose/River 新迁移。结算 worker 和批次详情读取会重新解析本站账号当前主角色，
历史批次即使保存过多个角色 ID，也只扫描、显示并发放给主角色。应用机服务
`active/ready`、数据库服务 active、重启次数 0；切换前备份为
`/var/backups/glorynavy/before-v0.1.0-contract-batch-main-recipient-20261003-20261003T111827Z.{dump,env}`。
公网前端上一版本保留为 `releases/v0.1.0-sentry-charging-icon-inline-20261003`；
容器静态入口、OpenResty `nginx -t`、首页/登录 200，批量结算详情匿名访问 401。
真实管理员登录后的批次 #6 页面和游戏合同交付仍需现场复核。

## 预警对账入口修复（2026-10-03）

Seat 的 `SENTRY_INTEGRATION_URL` 使用 `https://seat.kisectool.com` 同源入口。公网
OpenResty 必须把 `/api/v1/integrations/seat/` 整个前缀转发到 114 上的 EVE Sentry
服务；只转发密钥子路径会让预警投递、事件和监控贡献对账请求落回 Seat API 并返回
`404 接口不存在`。仓库来源为 [`deploy/edge-api-proxy.conf`](../deploy/edge-api-proxy.conf)，
变更后在公网机执行容器内 `nginx -t`，通过后平滑 reload，再用带服务令牌的只读
`monitor-contributions`、`alert-deliveries` 和 `alert-events` 请求验证 `200`。本次只
修复路由，不打开 EVE Sentry 的收费消费开关，也不创建真实扣币数据。

## 预警消费与收费配置（2026-10-03）

后端 `v0.1.0-sentry-charging-toggle-20261003`（Goose 64）和公网前端 `v0.1.0-sentry-charging-icon-inline-20261003` 已生产切换。页面提供余额、按小时价格、累计净消费、监控奖励和收费配置；收费状态图标嵌入配置按钮，默认关闭。应用 ready、首页/登录 200、匿名价格接口 401、静态入口及 OpenResty 检查通过。真实监控证据、价格复核和果壳币实账仍待现场验收。

## 预警空消费账单响应修复（2026-10-02）

生产前端已切换至 `v0.1.0-sentry-key-actions-20261002`，随后后端切换至 `v0.1.0-sentry-consumption-empty-20261002`。后端预警消费查询在没有记录时返回空数组，前端不再把合法空态判为响应格式异常；本轮无 Goose/River 新迁移，收费开关保持关闭。应用 `active/ready`、公网首页与登录 200、匿名模块和系统状态 401、静态容器入口及 OpenResty `nginx -t` 通过。前端上一版本和应用切换前备份均保留，可按现有回退约束恢复。

## 联盟 PAP 历史月份未兑换补兑（2026-10-02）

已发布 `v0.1.0-alliance-pap-history-20261002`。应用机与公网 1Panel 静态前端已原子切换，Goose 61、River 迁移和应用 `active/ready` 检查通过；切换前数据库与配置由激活脚本备份至 `/var/backups/glorynavy/before-v0.1.0-alliance-pap-history-20261002-20261001T171710Z.{dump,env}`。管理员可按完整历史月份预览并补兑联盟 PAP；历史兑换列表匿名返回 401。公网首页/登录 200、容器静态入口和 OpenResty `nginx -t` 通过，上一前端保留为 `releases/v0.1.0-operations-finance-20261001`。本轮用真实浏览器完成首页、登录入口和 EVE SSO 人物选择页只读检查，未提交授权；九月真实管理员操作、币账核对、实际补兑和授权后页面提交仍待现场验收。本机 Playwright 浏览器提交脚本因缺少 Chromium 可执行文件未运行。

## 审批批次编号恢复（2026-09-30）

公网静态前端已切换至 `v0.1.0-approval-batch-number-20260930`，批次主表首列恢复显示批次编号；游戏合同编号仍在详情复制区。仅更新静态前端，后端与数据库不变；公网首页、登录、匿名审批 401 和 OpenResty 配置检查通过。

## 合同接收角色文案收敛（2026-09-30）

公网静态前端已切换至 `v0.1.0-approval-main-recipient-label-20260930`，批次详情复制字段明确为合同接收角色 ID；后端保持上一版本，无数据库或服务行为变化。公网首页、登录、匿名审批 401 和 OpenResty 配置检查通过。

## 合并合同统一主账号接收（2026-09-30）

已发布 `v0.1.0-approval-main-recipient-20260930`。同一本站账号的多角色记录合并后，后台按账号主角色匹配游戏合同接收人；审批中心第二列显示主账号，详情复制字段为合同接收角色 ID。无 Goose/River 新迁移；应用 active/ready、Goose 57、公网首页/登录、匿名审批 401 和 OpenResty 配置检查通过。

## 批次审批表格整合（2026-09-30）

已发布 `v0.1.0-approval-batch-table-20260930`。后端与公网静态前端已原子切换；Goose 57、River 和应用 `active/ready` 检查通过。审批中心将活动批次直接显示在待发放主表，批次包含的来源单据不重复展示，详情提供候选接收角色、整数 ISK、结算编号和游戏合同 ID 的复制入口。生产旧批次清理前备份为 `/var/backups/glorynavy/before-settlement-cleanup-20260930T074538Z.dump`；本次发布备份由激活脚本保存在 `/var/backups/glorynavy/before-v0.1.0-approval-batch-table-20260930-*.{dump,env}`。公网首页、登录、匿名审批 401、新资源和 OpenResty `nginx -t` 均通过；真实管理员批次合同交付仍需现场验收。

## 同账号多角色合并合同（2026-09-30）

已发布 `v0.1.0-contract-batch-merged-20260930`。应用机与公网 1Panel 静态前端已原子切换，Goose 57、River 迁移和应用 `active/ready` 检查通过；切换前数据库与配置由激活脚本备份。审批中心创建批次时冻结同一本站账号下多角色的 ISK/物品快照，并生成 `BATCH-YYYYMMDD-UUID` 合同编号；River 按该编号核验实际接收角色、发放方、整数金额、物品多重集合和完成状态，再在一个事务内完成全部明细。公网首页、登录页、匿名受保护接口 401、容器内静态入口和 OpenResty `nginx -t` 检查通过。上一前端版本保留为 `releases/v0.1.0-contract-batch-account-group-20260930`；真实管理员创建游戏合同及多角色交付仍需现场验收。

## 批量结算按账号组隔离（2026-09-30）

已发布 `v0.1.0-contract-batch-account-group-20260930`。批量结算批次只允许同一本站账号组的记录，同组多个 EVE 角色可以一起处理，跨账号选择由前端和后端同时拒绝；无 Goose/River 新迁移。应用机与公网静态前端已原子切换，应用 active/ready、公网首页与登录 200、匿名模块接口 401、OpenResty `nginx -t` 检查通过。

## 合同批量结算等待态修复（2026-09-30）

已发布 `v0.1.0-contract-batch-retry-20260930`。批量结算在合同未接取或明细未同步时保留等待项，只有福利案件完成或兑换订单进入 `fulfilled` 才计入完成数；无 Goose/River 新迁移。应用机和公网静态前端已原子切换，应用就绪、公网首页/登录和匿名鉴权检查通过。

## 预警密钥单层卡片（2026-09-30）

已发布 `v0.1.0-sentry-single-card-20260930`。本次仅更新预警密钥页面的静态布局，应用 API 无行为变化、无 Goose/River 新迁移；应用机就绪、公网 OpenResty 配置和正式首页检查通过。

## 预警密钥自动准备（2026-09-30）

已发布 `v0.1.0-sentry-auto-key-20260930`。应用机 API 与公网静态前端已原子切换；本版本无 Goose/River 新迁移。每个本站账号只显示一张当前预警密钥卡片，首次打开页面自动准备默认密钥，已有密钥可通过“更新密钥”轮换；服务端先创建新远端密钥、再吊销旧远端密钥，失败时尝试回滚。切换前数据库与配置由激活脚本备份，应用服务 `active`/`ready`，公网首页、登录、匿名 401 和 OpenResty `nginx -t` 检查通过。真实成员轮换与远端双步确认仍需现场验收；上一前端版本保留为 `v0.1.0-sentry-key-rotate-20260930`。

## 预警平台密钥直连重试（2026-09-30）

已发布 `v0.1.0-sentry-api-retry-20260930`。应用机与公网前端均已切换，Goose/River 无新增迁移；激活脚本完成数据库与配置备份，应用服务 `active`/`ready`、重启次数为 0。密钥创建、创建结果不明后的重试、吊销和吊销重试均直接调用 EVE Sentry API；同一创建 `request_key` 与吊销操作保持幂等。上一版前端保留为 `/root/glorynavy-deploy/v0.1.0-sentry-api-retry-20260930.previous`。公网首页、登录和匿名 401 检查通过，OpenResty `nginx -t` 通过。

## QQ Bot 管理页（2026-09-30）

已仅更新公网静态前端至 `v0.1.0-qq-admin-20260930`。QQ Bot 与入群审批配置从 `/account` 移至管理员专用 `/community`，`/account` 保留个人 QQ/KOOK 资料；后端、数据库、Goose、River、会话和反代均未修改。切换前端上一版本保留为 `/root/glorynavy-deploy/v0.1.0-qq-admin-20260930.previous`。发布后容器内入口、OpenResty `nginx -t`、正式域名首页与 `/community` 返回 200，匿名私有接口返回 401。

## 合同批量结算（2026-09-30）

已发布 `v0.1.0-contract-batch-20260930`。应用机与公网前端均已切换，Goose 56、River 迁移和服务就绪检查通过；数据库与配置备份位于应用机 `/var/backups/glorynavy/before-v0.1.0-contract-batch-20260930-20260929T174208Z.{dump,env}`。上一版本为 `v0.1.0-seat-20260930`，仍保留用于回退。公网首页、登录页、审批路由和匿名鉴权检查通过；浏览器 SSO/PKCE/CSRF 验收脚本使用本机 Chrome 通过。真实管理员批量结算与游戏合同交付仍需现场验收。

## 上一版本（2026-09-30）

已发布 `v0.1.0-seat-20260930`。应用机与公网前端均已切换，Goose 55、River 迁移和服务就绪检查通过；数据库与配置备份位于应用机 `/var/backups/glorynavy/before-v0.1.0-seat-20260930-20260929T164704Z.{dump,env}`。上一版本为 `v0.1.0-pap-supplement-20260929`，仍保留用于回退。公网首页、登录页和匿名鉴权检查已通过。

## 补录后补发集结分（2026-09-29）

应用与公网前端已原子切换至 v0.1.0-pap-supplement-20260929。无 Goose/River 迁移，数据库保持 Goose 54；应用机备份为 /var/backups/glorynavy/before-v0.1.0-pap-supplement-20260929-20260929T080017Z.{dump,env}。应用服务 active、ready、NRestarts=0，健康端点返回 200；正式域名首页与 /login 返回 200，匿名受保护系统状态返回 401。公网前端上一版本保留为 releases/v0.1.0-qq-attendance-20260929。

## QQ Bot 配置与考勤补录（2026-09-29）

应用与公网前端已原子切换至 v0.1.0-qq-attendance-20260929。应用机 /opt/glorynavy/current 与公网机 1Panel 站点 current 均指向新版本，前端上一版本保留在 /root/glorynavy-deploy/v0.1.0-qq-attendance-20260929.previous。激活脚本在切换前完成数据库 dump 与 /etc/glorynavy/seat.env 备份，备份文件位于应用机 /var/backups/glorynavy/before-v0.1.0-qq-attendance-20260929-20260929T064158Z.{dump,env}；迁移完成后数据库为 Goose 54。

发布后应用服务 active、ready、NRestarts=0；应用机健康端点 200，/api/v1/system/status 与 QQ Bot 设置接口匿名均为 401。正式域名首页和 /login 返回 200，匿名系统状态返回 401；公网 OpenResty 容器内静态入口和 nginx -t 检查通过。真实 QQ 平台回调联调仍需在 /account 配置开发者应用和群 OpenID。主动 QQ 推送通知不在本次版本范围。

## 活动福利多角色申请（2026-09-28）

应用与公网前端已原子切换至 `v0.1.0-activity-multi-20260928`。主账号可在一次申请中选择多个已绑定角色，每个角色独立生成福利案件、截图快照、审批状态和合同核验目标；角色级确定性幂等键保证重试不重复建单。无 Goose/River 迁移。应用机激活脚本已完成数据库与配置备份，应用服务 `active`/`ready`；公网首页、登录状态接口返回 200，匿名受保护接口返回 401。上一版前端仍保留在公网机发布归档中，可按相对 `current` 软链接回退。自动 Playwright 检查受本机缺少可用 Chromium 运行时影响，发布后以静态资源、HTTP 和服务健康检查完成验收。

## 全局 Toast 通知前端发布（2026-09-28）

公网前端已原子切换至 `releases/v0.1.0-toast-20260928`，上一版保留为 `releases/v0.1.0-welfare-quota-chart-20260928`。本次只替换静态资源，API 未重启、无 Goose/River 迁移；容器内入口、新资源、匿名审批接口 401 和 Chrome 浏览器登录流程检查通过。回退使用 `/root/glorynavy-deploy/v0.1.0-toast-20260928/previous-frontend` 中的相对软链接目标。

## 补损额度图表与审批读取优化（2026-09-28）

应用与公网前端已切换至 `v0.1.0-welfare-quota-chart-20260928`。本次无 Goose/River 迁移，数据库保持 Goose 49；应用机激活脚本已自动备份数据库与配置，公网前端上一版本记录在 `/root/glorynavy-deploy/v0.1.0-welfare-quota-chart-20260928/previous-frontend`。补损额度卡片显示已用/待审批预占分段图；审批来源权限、上下文和队列查询并行执行，前端只加载当前视图。服务 active、ready、NRestarts=0，正式域名首页、登录、福利、审批路由及新资源 200，匿名审批接口 401。回退前需先检查新增待审批预占与额度配置，前端使用相对软链接切换。

## 审批排序游标与待领取状态修复（2026-09-28）

应用与公网前端已切换至 `v0.1.0-approval-fix-20260928`。本次无 Goose/River 迁移；应用机和 1Panel 站点均使用新版本，旧前端链接保存在公网机 `/root/glorynavy-deploy/approval-fix-20260928.previous`。发布后服务 active、ready、NRestarts=0，首页和 `/approvals` 返回 200，匿名审批接口返回 401。回退沿用相对软链接约束。

## 审批中心排序发布（2026-09-27）

应用与公网前端已切换至 `v0.1.0-approval-sorting-20260927`。本次无 Goose/River 迁移；应用机 `/opt/glorynavy/current` 和 1Panel 站点 `current` 均使用新版本，前端软链接保持容器可解析的相对路径。发布后应用服务为 active、ready、NRestarts=0，公网首页及 `/approvals` 返回 200，匿名受保护审批接口返回 401。版本归档、前端上一版本链接和切换证据保存在应用机与公网机的 `/root/glorynavy-deploy/`。

## 活动福利截图提交修复（2026-09-26）

生产公网 OpenResty 原站点配置中的 `client_max_body_size 1m` 会在活动截图请求到达 API 前返回 413。已备份 `/opt/1panel/www/conf.d/zz-glorynavy-seat.conf`，将该站点上限调整为 `8m`，执行容器内 `nginx -t` 并平滑 reload；后端接口仍限制总请求 7 MiB、单张 2 MiB、最多 3 张。1.5 MiB multipart 回归请求已不再被边缘返回 413，而是正常到达鉴权层并返回 401。未修改应用二进制、数据库迁移或会话。

随后发现 Shu Tur5 的提交在通过反代后仍会等待福利全局事务锁约 15 秒并返回 400，公网日志对应 499。后端已改为活动申请使用独立事务锁，避免合同交付/补损审核扫描阻塞新申请；本轮需同步发布 Go 服务，Goose/River 无迁移。

## 补损审批金额与周期额度发布（2026-09-25）

前后端 `current` 已切换至 `releases/v0.1.0-welfare-quota-20260925`，Goose 48→49；生产库副本迁移预检通过。应用机升级前数据库与配置备份在 `/var/backups/glorynavy/v0.1.0-welfare-quota-20260925/`，后端上一版为 `releases/v0.1.0-online-accuracy-20260924`；公网机前端上一版记录在 `/root/glorynavy-deploy/welfare-quota-20260925/previous-frontend`，为 `releases/v0.1.0-member-page-prefetch-20260925`。公网静态资源保留旧 hash 文件。Goose 49 只增加索引，未自动设置任何补损额度；发布后 5 条福利策略均未启用周期额度。

服务 active、Goose 49、账号/福利单/会话数量与切换前一致；正式域名主要路由 200、匿名受保护端点 401，真实浏览器登录表单及 SSO/CSRF/PKCE/scopes 检查通过。真实管理员的额度设置与实际超额审批仍需现场验收。回退应先检查新增策略配置与审批记录；旧后端不了解新额度，不能在已启用额度后直接切回旧二进制。前端独立回退仍使用容器可解析的相对软链接。

## 成员页请求时序前端发布（2026-09-25）

公网前端 `current` 为 `releases/v0.1.0-member-page-prefetch-20260925`，上一版 `releases/v0.1.0-approval-navigation-prefetch-20260925` 保留并记录在公网机 `/root/glorynavy-deploy/v0.1.0-member-page-prefetch-20260925.previous`。本次只更新静态前端，保留旧哈希 assets；归档 SHA-256 为 `B457B205F6493F8F20451EEDDA7E4D8E751D88682D016E8BB6034CC8D38A577F`，新 `index.html` SHA-256 为 `A1DEFE4097DDCDF0D28360E443E9B91FBB45E02D6B62028D8AC3FDF7C814161F`。切换使用容器可解析的相对软链接；源站与正式域名主要页面、新资源、匿名 401 及真实浏览器登录/CSRF 检查通过。后端、Goose 48 和 1Panel 代理未改。

## 审批分类与分页预读前端发布（2026-09-25）

公网前端 `current` 为 `releases/v0.1.0-approval-navigation-prefetch-20260925`，上一版 `releases/v0.1.0-workspace-approval-fast-20260925` 保留并记录在公网机 `/root/glorynavy-deploy/v0.1.0-approval-navigation-prefetch-20260925.previous`。本轮只覆盖新静态产物，旧哈希 assets 继续保留；`current` 使用容器可解析的相对链接。源站和正式域名的首页、工作台、审批中心及新资源均为 200，匿名受保护 API 为 401，真实浏览器 EVE 登录跳转与 CSRF 检查通过。后端、Goose 48 和 1Panel 代理未改。

## 工作台审批首载前端发布（2026-09-25）

`v0.1.0-workspace-approval-fast-20260925` 已发布，现为本轮回退目标；归档及其上一版路径记录在公网机 `/root/glorynavy-deploy/`。该版仅替换静态前端，后端 `v0.1.0-online-accuracy-20260924`、Goose 48 和 1Panel 代理不变。构建基线临时回退该单处改动后，108 个静态产物名与当时生产完全一致，确认该次构建只包含工作台并行请求改动；旧哈希资源随新目录保留。首次切换时用了宿主机绝对路径软链接，1Panel OpenResty 容器内无法解析，静态路由短暂 500；已回退并改用相对链接重新切换，当次源站/正式域名的首页、工作台、登录及新资源均为 200，匿名受保护 API 为 401，浏览器登录与 CSRF 检查通过。

**前端切换约束**：`current` 必须是相对于 `/opt/1panel/www/sites/seat.kisectool.com/` 的 `releases/<版本>` 软链接。宿主机绝对路径 `/opt/1panel/...` 在容器的 `/www/sites/...` 中不存在。原子切换前后均以 `docker exec 1Panel-openresty-ybA0 test -f /www/sites/zz-glorynavy-seat/index/index.html` 检查容器内路径，再检查 HTTPS 首页；回退也使用相对链接。

## 审批中心前端发布（2026-09-25）

公网机前端 `current` 已原子切换至 `releases/v0.1.0-approval-prefetch-20260925`；上一版路径保存在 `/root/glorynavy-deploy/approval-prefetch-20260925.previous`，旧版 `releases/v0.1.0-data-priority-20260924` 及旧哈希 assets 均保留。前端归档校验值和构建入口文件校验值已在上传、解包和切换后核对；生产首页、登录、审批路由、新静态资源、匿名 401 及真实浏览器 EVE 登录跳转检查通过。回退只需将公网机 `current` 原子切回上述上一版，不重启应用机 API，不执行迁移或改动 1Panel 反代。后端仍为 `v0.1.0-online-accuracy-20260924`，Goose 48 不变。真实管理员切页耗时仍需登录后复测。

## 当前 1Panel 管理位置（2026-09-25）

在**公网机 `47.243.104.165` 的 1Panel**“网站”列表选择 `seat.kisectool.com`（代号 `zz-glorynavy-seat`）；应用机 `10.233.53.209` 的 1Panel 不托管此网站。网站类型是“静态网站”，因为公网机直接提供 React 前端文件；同一网站“配置 → 反向代理”中的 `api` 规则将 `/api/` 送往名为 `glorynavy_api` 的 Nginx 上游。这个名称不是公网域名；在该网站的“负载均衡”中可看到唯一成员为 ZeroTier 地址 `10.233.53.209:18080`，并保留 keepalive 连接复用。它不是整站反向代理网站，不能把首页转发到仅提供 API 的 `18080` 端口。生产主配置为 `/opt/1panel/www/conf.d/zz-glorynavy-seat.conf`，仓库来源为 [`deploy/edge-https.conf`](../deploy/edge-https.conf)；代理规则实际文件为 `/opt/1panel/www/sites/zz-glorynavy-seat/proxy/api.conf`，仓库来源为 [`deploy/edge-api-proxy.conf`](../deploy/edge-api-proxy.conf)；负载均衡文件为同站点的 `upstream/glorynavy_api.conf`，仓库来源为 [`deploy/edge-api-upstream.conf`](../deploy/edge-api-upstream.conf)。规则保留 `/api/` 的安全响应头与不缓存设置；其中 `/api/v1/integrations/seat/` 先于通用 `/api/` 规则转发到 114 上的 EVE Sentry，其他 API 继续送往 `glorynavy_api`。修改时同步这三份仓库文件，检查 `nginx -t` 后 reload。不要重新创建同域名网站或恢复旧 `seat.kisectool.com.conf`，以免重复 server。面板站点 `/opt/1panel/www/sites/zz-glorynavy-seat/index` 是指向原 `/opt/1panel/www/sites/seat.kisectool.com/current/web` 的软链接，因此前端发布仍切换原 `current` 链接。发布包在应用机的 `releases/<版本>/web` 中也包含前端文件，但当前生效的 OpenResty 根目录不是该副本；仅切换应用机 `current` 不会更新用户看到的网页。站点访问/错误日志在面板站点 `log/` 目录，访问日志不含查询字符串或 Cookie。

现有证书由 Certbot 管理，不要在面板里另行申请同域名证书。证书已作为手动证书登记在面板，续期钩子 [`deploy/renew-seat-certificate.sh`](../deploy/renew-seat-certificate.sh) 同步原证书目录和面板网站 `ssl/` 目录，再验证/reload OpenResty；钩子已使用现有证书执行并通过。面板手动证书记录的到期日不会随 Certbot 自动续期更新，实际服务证书以 `ssl/` 文件和 HTTPS 握手为准，续期后应更新面板记录。迁移备份位于公网机 `/root/glorynavy-deploy/1panel-site-20260925/`，含原配置、面板数据库快照与切换前生成配置。前后端版本和 Goose 48 未变。

如需回退这次站点接管，先将面板配置文件移出 `conf.d`，把上述备份中的 `seat.kisectool.com.conf.deactivated` 还原到原路径，执行 `nginx -t` 和 reload，并检查首页、登录、API 与 ACME challenge；确认旧入口恢复后再通过 1Panel 删除新登记的站点。仅需撤销代理规则拆分时，恢复备份中的 `zz-glorynavy-seat.conf.before-panel-proxy` 并停用 `proxy/api.conf`，避免重复 `/api/` location。不要在运行中的面板上直接覆盖整个 `agent.db`，也不要删除原 `seat.kisectool.com` 发布目录。迁移时的 HTTP challenge 已在本机以实际临时文件验证为 200。

## 站点反代耗时日志与 JSON 压缩（2026-09-24）

公网机 1Panel OpenResty 的 `/opt/1panel/www/conf.d/seat.kisectool.com.conf` 已与仓库 `deploy/edge-https.conf` 同步。新增 `glorynavy_safe` 耗时日志格式，仅记录 HTTP 方法和 URI 路径及 Nginx/上游耗时，不记录 query、Cookie 或 OAuth code；站点 gzip 覆盖较大的 `application/json`，保留原 JS/CSS 压缩与 `/api/` 不缓存。修改前配置备份位于公网机 `/root/glorynavy-deploy/edge-observe-20260924/seat.kisectool.com.conf.before`。`nginx -t`、首页 200、私有模块目录匿名 401、公开活动接口 gzip 已核对。当前前端 `v0.1.0-data-priority-20260924`、后端 `v0.1.0-online-accuracy-20260924`、Goose 48 均未切换。应用机 1Panel 自带 OpenResty/PostgreSQL 不在本站生产请求路径，不应将本站数据库参数改到那套容器上。

**当时的管理方式**：2026-09-24 此网站仅由手工 `conf.d` 文件加载，尚未登记 1Panel“网站”；此限制已在 2026-09-25 迁移后解决。当前管理位置以上节为准。

## 当前页面数据优先（2026-09-24）

生产前端已切换到 `v0.1.0-data-priority-20260924`。后台菜单预热在页面业务请求未完成时让路，已打开页面继续按原规则获取数据。上一前端保留在 `releases/v0.1.0-data-firstload-20260924`；后端与 Goose 48 不变，反代上游 keepalive 配置保持生效。

## 数据首载与反代连接复用（2026-09-24）

生产前端已切换到 `v0.1.0-data-firstload-20260924`；后端仍为 `v0.1.0-online-accuracy-20260924`，Goose 48 不变。`seat.kisectool.com.conf` 通过 `glorynavy_api` upstream 复用到 `10.233.53.209:18080` 的连接，仍不缓存 `/api/`。修改前配置备份位于公网机 `/root/glorynavy-deploy/api-keepalive-20260924/seat.kisectool.com.conf.before`，前端上一版本保留在 `releases/v0.1.0-menu-warmup-20260924`；配置已通过 `nginx -t` 和路由/匿名鉴权检查。真实成员页面与 CDN 端到端耗时仍待量测。

## 仅前端更新：成员菜单空闲预热（2026-09-24）

生产前端已切换到 `v0.1.0-menu-warmup-20260924`，后端仍为 `v0.1.0-online-accuracy-20260924`，Goose 48 不变。新前端在登录及权限确认后顺序预热可见菜单的代码；业务数据仍在进入页面后读取。静态资源保留旧版 assets 供已打开页面使用，前端回退目标为 `releases/v0.1.0-online-accuracy-20260924`。生产路由和匿名接口 HTTP 检查通过，真实登录成员首次加载及 Edge 快速切页待复测。

## 已发布：在线准确性、活动福利及本地改进（2026-09-24）

前后端已切换至 `v0.1.0-online-accuracy-20260924`，Goose 47→48。生产快照先在独立库演练迁移，正式切换前备份数据库与配置并核对业务摘要；真实浏览器登录跳转、匿名隔离、服务就绪及新在线采样间隔通过。同步发布活动福利、损失详情、PVP 异步核价、前端按需加载与工作台钱包汇总；OpenResty `/images/` 一天缓存也已启用。真实活动福利及合同交付、成员在线统计页面仍待游戏/玩家会话联调。发布证据见[项目状态](project-status.md)与[在线准确性排查](reviews/production-online-accuracy-2026-09-24.md)。

活动福利截图上传需要反向代理允许至少 `8m` 请求体；接口本身仍限制总请求 7 MiB、单张 2 MiB。若代理保留默认 `1m`，较大的截图会在到达 API 前返回 HTML 413，前端只能显示“提交失败”。修改 OpenResty 配置后先执行 `nginx -t`，再平滑 reload，并用未登录的 1.5 MiB multipart 请求确认代理不再返回 413。

## 已发布：合同整数 ISK 金额（2026-09-23）

前后端已切换至 `v0.1.0-contract-whole-isk-20260923`，Goose 47 不变。游戏合同复制金额和自动核验均向下取整到整数 ISK，历史精确小数付款兼容；原核准金额与快照不重算。已备份数据库和配置，核对业务摘要；公网资源、匿名权限和真实浏览器登录跳转通过。游戏内真实合同仍待 FUL-01 联调。

## 已发布：奖励组合与定期核价（2026-09-23）

生产已备份并升级至 Goose 47，前后端版本 `v0.1.0-mixed-rewards-20260923`；原配置、业务/会话摘要一致。后续发布仍需备份数据库/配置、停止旧 API/worker、执行 Goose/River 迁移并切换匹配产物。新增 `exchange.reward-price.v1` 使用现有 River exchange 队列，每分钟扫描到期奖励，六小时更新一次；实物核价依赖 market。既有奖励迁移为手动定价，联盟 PAP 兑换模式不因升级自动改变。

回退前停止调度并取消新核价 kind 的未完成任务。有 ISK 奖励快照或联盟 PAP 合并归属后，不能回退到忽略这些字段的程序/迁移。匿名验证应确认 `/api/v1/modules`、`/api/v1/system/status`、`/api/v1/eve/status` 返回 401；登录页使用仅含 configured 的 `/api/v1/eve/login-status`，就绪探针仅返回 status。真实游戏混合/纯现金合同仍待 FUL-01 验证。


适用于当前两台 Ubuntu 24.04 amd64 主机。部署与真实玩家登录的验收状态见[项目状态](project-status.md)；本文不把构建成功等同于网站已上线。

首版 v0.1.0 已于 2026-09-14 开放 HTTPS 站点。当前入口为 `https://seat.kisectool.com`，生产使用独立 EVE 开发者应用和全新数据库；真实玩家登录、管理员首次设置及证书演练的详细状态以部署记录为准。

## 拓扑与目录

当前前后端均为 `v0.1.0-welfare-quota-20260925`，Goose 49。旧版产物与静态 assets 保留；本轮迁移及缓存配置变更见上文。详细验收及备份见[项目状态](project-status.md)，历次发布证据见[交付历史](history/project-status-through-2026-09-23.md)。联盟 PAP 每 30 分钟完整读取当前月快照后差量落库；支持手动与自动增量兑换，生产保留既有 manual 配置。

`浏览器 → https://seat.kisectool.com → 公网 OpenResty → ZeroTier → Go API → PostgreSQL`

- 公网机 `47.243.104.165`，ZeroTier `10.233.53.17`：复用现有 1Panel OpenResty 容器 `1Panel-openresty-ybA0`，托管 React 静态产物并终止 HTTPS。
- 应用机 `10.233.53.209`：Go API 监听 `10.233.53.209:18080`，独立 PostgreSQL 容器仅发布 `127.0.0.1:55433`。最初提供的 `.207` 已更正为 `.209`。
- 应用版本 `/opt/glorynavy/releases/<版本>`，活动链接 `/opt/glorynavy/current`；可写 SDE `/var/lib/glorynavy/sde`。生产不运行 Vite 或安装 Node/Go 编译器。
- 配置 `/etc/glorynavy/seat.env` 为 root 所有、0600；systemd 读取环境后以专用 `glorynavy` 用户运行应用。数据库引导密码 `/etc/glorynavy/db-password` 位于 root 专用 0700 目录，文件 0444 以便容器内 PostgreSQL 用户读取 secret 挂载，宿主普通用户不能遍历父目录。应用数据库账号单独生成密码，不具有超级用户权限。配置样例见[seat.env.example](../deploy/seat.env.example)。
- 前端 `/opt/1panel/www/sites/seat.kisectool.com/releases/<版本>/web`，活动链接位于同目录 `current`；容器内对应 `/www/sites/seat.kisectool.com`。
- 本项目 nftables 独立表只限制 API 端口，允许本机和反代机。不得清空宿主的现有防火墙或 Docker 规则。

## 构建与安装

在已安装本地开发依赖的仓库执行（版本目录必须不存在）：

```sh
node scripts/build-release.mjs v0.1.0
```

产物在 `.local/releases/v0.1.0.tar.gz`，含 5 个 Linux 静态二进制、前端、部署模板、版本元数据与 `SHA256SUMS`，不包含 `.env`、数据库、SSH 密钥或源码依赖。通过已有 SSH 私钥上传到应用机，解压至版本目录，然后执行：

```sh
bash /opt/glorynavy/releases/v0.1.0/deploy/bootstrap-app.sh
# 安全配置 seat.env 和 db-password 后执行；不要把真实配置贴入命令记录。
bash /opt/glorynavy/releases/v0.1.0/deploy/activate-release.sh v0.1.0
```

激活脚本验证校验和，安装并启动专用数据库与网络规则，停止旧 API，保存数据库 dump 和原配置，再切换活动链接。服务启动先执行 Goose 与 River 两条迁移；不跳过队列迁移。数据库镜像固定为已检查的 PostgreSQL 18.6 Alpine digest，独立 Compose project `glorynavy-production` 与 volume，避免复用其他业务库。

首次创建的生产库与本地开发库分离。若选择迁移本地业务数据，先停止本地 API/worker，备份并迁移匹配的 `EVE_TOKEN_KEY` 和数据库；不得让两边同时刷新同一批 EVE refresh token。恢复后清除旧浏览器会话与未完成的 OAuth 登录事务，再检查 River 任务恢复。此操作需按实际数据选择执行，不能假定已迁移。

## 域名、证书与反代

DNS 添加 `seat` A 记录指向公网机；初次联调使用仅 DNS。没有正确的 AAAA 地址时不要配置 AAAA。EVE 开发者应用登记回调：

```text
https://seat.kisectool.com/api/v1/eve/callback
```

后端 `PUBLIC_ORIGIN=https://seat.kisectool.com`，正式环境 Cookie 自动启用 Secure；OAuth、Origin 与 CSRF 校验保持开启。SeAT 兼容基线为 57 项 scopes；启用 fittings 时还需 `esi-fittings.write_fittings.v1`，共 58 项，旧角色重新授权后才能写游戏方案。详见[授权清单](integrations/seat-login-scopes.zh-CN.md)。

**以下为全新部署的证书引导步骤，现有生产不要重跑。** 首次签发时曾将 [HTTP challenge 配置](../deploy/edge-http.conf) 暂放 `/opt/1panel/www/conf.d/seat.kisectool.com.conf`；迁入 1Panel 后该临时文件已停用。ACME webroot 仍为宿主 `/opt/1panel/www/sites/seat.kisectool.com/acme`。改动现有站点前先备份，执行容器内 `nginx -t` 通过再 reload，不覆盖其他站点配置。

```sh
certbot certonly --webroot \
  -w /opt/1panel/www/sites/seat.kisectool.com/acme \
  -d seat.kisectool.com --cert-name seat.kisectool.com
```

复用服务器既有 ACME 账户；首次账户按运维联系方式登记。新证书私钥只在公网机生成。安装[证书发布钩子](../deploy/renew-seat-certificate.sh)到 `/etc/letsencrypt/renewal-hooks/deploy/glorynavy.sh`（0755）；首次用 `RENEWED_DOMAINS` 和 `RENEWED_LINEAGE` 指向新证书执行一次，复制证书到 OpenResty 已挂载目录。

证书、前端和 API 就绪后使用 [HTTPS 配置](../deploy/edge-https.conf)，当前生产安装目标是 1Panel 网站 `zz-glorynavy-seat` 的 `conf.d/zz-glorynavy-seat.conf`。HTTP 保留 challenge 路径，其余跳转 HTTPS；前端路由支持刷新，API 不走 SPA 回退、不缓存。哈希 `/assets/` 缓存一年，固定文件名 `/images/` 缓存一天；更新图片后最长可能等待一天自然过期。专用访问日志只记路径，不记录 OAuth query、Cookie 或 referrer。

HTML 页面的 `Referrer-Policy` 使用 `same-origin`，使同源原生登录表单 POST 保留正确 Origin。不能在页面上统一设置 `no-referrer`：浏览器可能发送 `Origin: null`，与严格 CSRF 校验冲突。API（含 OAuth 回调）继续使用 `no-referrer`，防止后续导航带出回调参数；不要通过接受 null Origin 或在反代中伪造 Origin 来绕过。参见[MDN 来源策略](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Referrer-Policy)。

现有 `certbot.timer` 执行续期，成功后钩子复制证书并验证/reload OpenResty。上线时执行 `certbot renew --cert-name seat.kisectool.com --dry-run` 验证续期链路；不重复安装系统定时器。[Certbot 官方说明](https://certbot.eff.org/instructions?os=pip&tab=wildcard&ws=nginx)、[NGINX 反代契约](https://nginx.org/en/docs/http/ngx_http_proxy_module.html)。

需要包含发布钩子的人工演练时，追加 `--run-deploy-hooks --no-random-sleep-on-renew`；正常定时续期保留随机等待。目录钩子必须筛选 `RENEWED_DOMAINS`，避免其他站点的钩子误处理本站。测试 CA 返回限流时遵循 Retry-After，不循环请求；模拟签发、钩子执行和正式证书有效性分别记录。

## 验收、管理员与恢复

检查服务 active、目标版本 Goose/River 迁移、内网 `/health/ready`、公网页面及静态资源、匿名接口、未授权业务 401、跨站登录拒绝，以及 EVE 登录跳转包含正确回调和 Secure flow Cookie。真实 CCP 玩家登录及授权仍需浏览器完成，跳转成功不等于登录已验证。

发布后必须验证真实浏览器按钮提交，不能仅以按钮可用或手动附带 Origin 的 HTTP 请求代替：

```sh
node scripts/check-deployment.mjs https://seat.kisectool.com
```

脚本使用 web 的 Playwright 依赖；可通过 `PLAYWRIGHT_CHROMIUM_EXECUTABLE` 指定已安装 Chrome。它真正提交生产登录表单，检查浏览器 Origin、303、PKCE/scopes、Cookie、API 来源策略和越权拒绝；仅在即将到达 CCP 时截停，不输入玩家密码、不完成授权。每次运行创建一个正常的短期登录事务，不应高频循环执行。

脚本核对匿名元数据拒绝、最小登录投影以及实际 SSO 跳转的 57/58 项 scope。当前版本需 Goose 49 和 River；表的存在不代表业务模块已开启。当前 MODULES 为 `system,identity,eve,access,community,fittings,skills,attendance,exchange,welfare,wallet,market,approval`，工作台管理员审批待办已启用。本次不增加登录 scope，启用不补发历史 PAP 或果壳币；默认手动兑换，管理员需设置比例、ISK 估值和奖励。生产奖励库未导入本地配置，需要管理员自行建立并上架。

配装版本回退至旧同步版本前，先停止新 API/worker，阻塞 fittings/skills/skillqueue 目标并取消旧程序不识别的 `eve.fitting-resource.v1` 未完成任务，再恢复旧 MODULES 和后端活动链接。保留新增表，不执行破坏性 Down；前端独立切回旧版本。升级备份需校验 dump 可读取，并保存配置、旧版本路径和产物校验值。

检查从 `/` 开始：公开军团介绍应保留在首页，匿名访问 `/workspace` 才进入 `/login`；再验证右上角表单提交。前端单独更新时使用新的静态版本目录并原子切换 `current`，保留上一版带 hash 的 assets 以兼容已经打开的页面；不为纯前端变更重启 API 或重新迁移数据库。

已指定的管理员为 Hajimi1。全新数据库中先由该角色正常登录，再通过安全加载服务器环境运行 `current/bin/access-admin --character 已核对的角色ID`；不得自动把第一个登录者设为管理员，也不把名字当作已验证角色归属。

当前生产环境已按用户指定完成 Hajimi1、Nuter Zero 的有效绑定核对与本站超级管理员授予，具体操作记录见[交付历史](history/project-status-through-2026-09-23.md)。上述首次设置步骤保留用于重建环境，不应在每次发布时重复授予。

```sh
systemctl status glorynavy glorynavy-database glorynavy-network
journalctl -u glorynavy --since '10 minutes ago'
curl --fail http://10.233.53.209:18080/health/ready
```

每次激活自动在 `/var/backups/glorynavy` 留存升级前 dump 与配置（0700/0600），操作员需另做受保护的异机备份。现有记录未提供定期数据库备份及恢复演练的完成证据，需按[OPS-01](backlog.md)只读核实任务并补齐；不把升级备份当作完整灾备。前端回退可切换 `current`；后端回退先停服务、判断 schema 兼容，不盲目 Goose down。不兼容时将匹配的 dump 和令牌密钥恢复至独立库验证，再切换，不覆盖其他业务库。

## 历史模块发布与兼容边界

2026-09-21 已发布前端 `v0.1.0-growth-history-20260921`：成长福利历史领取移除核验依据输入，自动记录操作摘要并保留历史审计。后端仍为 `v0.1.0-welfare-capital-20260921-r2`、Goose 40，未重启服务、未迁移或清理会话。发布前构建/lint及桌面/手机两项历史保存回归通过；发布后 6 个路由、84 个静态文件哈希及 3 个匿名接口隔离检查通过，真实浏览器登录跳转和安全校验通过。上一前端及其 assets 保留，可切回 `releases/v0.1.0-welfare-capital-20260921-r2`；证据 `.local/deployment/growth-history-20260921/`。

2026-09-21 已发布 `v0.1.0-welfare-capital-20260921-r2`（Goose 40）：成长福利、旗舰合同 ISK 补贴与独立可配置比例、绑定即有效、兑换取消审核与合同核对、随机结算编号及页面优化。当次生产备份和迁移前后业务/会话摘要一致，公网资源与登录检查通过。原配置保留，无本地业务数据导入。恢复需兼容现有交付记录、随机编号和比例快照，不能直接回退旧二进制；详见[项目状态](project-status.md)。

2026-09-20 已发布 `v0.1.0-reward-library-20260920`（Goose 37），统一前后端版本，启用福利、钱包、估价与共享奖励库。停服备份、Goose/River 迁移、账号与账本不变量、静态资源校验、真实浏览器登录跳转及匿名隔离检查通过，证据和备份位置见[项目状态](project-status.md)。未代用户建立奖励或进行真实兑换/交付。回退还须考虑 killmails、钱包及福利交付核验新作业，先按各模块指南处理旧程序不认识的任务；不得仅切换旧二进制或对业务历史执行 Down。

2026-09-16 曾发布 `v0.1.0-account-merge-fixes-20260916`（Goose 30），包含账号合并、技能历史快照保留/达标检查及公共弹窗统一。模块和 scopes 不变，备份、产物一致性、登录及匿名隔离验证通过，详细证据见[交付历史](history/project-status-through-2026-09-23.md)。首次真实合并由成员自行完成双账号 SSO 及明确确认。出现完成合并记录后，旧二进制可能不理解已停用来源账号，禁止自动回退旧程序或普通 Down；须保留追溯数据并使用兼容程序恢复。

2026-09-15 已发布 `v0.1.0-fitting-library-20260915`，随后升级到 `v0.1.0-sync-queue-fix-20260915`，修复训练队列导致同步列表读取失败及正常等待误记失败。18:06 在该产物上启用 attendance/exchange，已备份数据库和原配置，生产 Goose 29 不变。仅停用这两模块时，恢复启用前 MODULES 并重启；当前版本认识相关作业，在途任务休眠、已有数据保留，不需 Down。回退到不认识 online 或 attendance 作业的更旧程序必须按[考勤指南](integrations/attendance.zh-CN.md)清理非终态作业并封存目标，不能只切二进制。用户确认开发者应用写 scope；玩家授权和真实业务验收边界、备份及验证详情见[项目状态](project-status.md)。

## 同步清理性能升级（Goose 46，2026-09-23 已发布）

先完成 Goose 46 的两个并发索引，再发布后端；无需更换前端或重启数据库。迁移可重试，失败残留索引会重建。DB_SLOW_QUERY_MS 默认 500，随新版主服务生效，0 关闭；不会记录参数和 SQL 原文。未自动开启 pg_stat_statements。验证与回退见[数据库性能观测](database-performance.md)。

## 公开军团首页发布（2026-09-23）

后端 v0.1.0-public-home-20260923、前端 v0.1.0-public-home-20260923-r1，Goose 46 不变。新 /api/v1/eve/public/corporation 仅投影固定军团的官方匿名资料；私有业务仍鉴权。备份和验证见[项目状态](project-status.md)。

本轮临时发布脚本复用既有服务、数据库和反代，未覆盖配置或重启数据库；保留旧 assets。首页 / 不再自动跳转登录，工作台改为 /workspace。没有会话清理。发布验证需区分 HTTP 产物校验与真实浏览器提交，浏览器连接失败应明确记录待验证项。

## 2026-09-23 首页实力数字发布

前后端版本 v0.1.0-public-strength-20260923-r1，Goose 保持 46，配置与会话不变。数据库/配置备份、静态资源哈希和匿名接口校验已完成，详细验收与浏览器复核限制见 [项目状态](project-status.md)。固定军团新增公开战绩/在线角色聚合，页面只保留四项名称＋数字，不显示图表和说明小字。
## 预警收费开关同步（2026-10-03）

Seat 管理员保存 `/sentry` 收费开关时，后端先用服务令牌调用 EVE Sentry 的
`PUT /api/v1/integrations/seat/alert-consumption`，预警端持久化门禁后才提交本地价格事务。
远端失败不会留下“页面已开启、预警端仍关闭”的半状态；发布后需验证该接口的 200 响应和预警端重启后的持久化读取。
