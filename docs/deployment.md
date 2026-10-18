# Linux 生产部署

## 2026-10-19 统一角色与名称范围选择

前后端 `v0.1.0-role-scope-20261019` 来自干净且已推送 main 提交 `e12133d09d72b7e1195b355a0c46c23354db2127`。移除专员快捷按钮，使用通用角色创建／成员授权；军团和联盟通过受 `access.manage` 保护的只读 `/api/v1/access/scope-options` 接口按名称多选。选项限配置军团及当前可见范围和有效快照，联盟按所属去重；缺失选项不清空旧范围，角色保存及业务权限判定沿用原实现。

完整 access 测试、4 项宿主权限／元数据测试（race）、go vet 通过；类型／构建／受影响 lint、7 项前端单测、18 项中英文 1280/375/320px 浏览器流程和截图检查通过。初次重复词条、残余 import 和测试定位问题已修正并重测。GitNexus 提交前检测报 CRITICAL，混入多模块同名符号；已告知并按实际文件复核调用范围，宿主仅注入名称服务、模块仅新增只读路由，原角色保存／审批逻辑未修改。

生产构建的 138 个前端文件与已测产物一致，应用机从干净 main 编译 6 个 Linux 程序，前后端来源一致、SHA256 清单通过。先按标准流程备份并激活后端，再切换边缘静态文件；Goose 98、River 和环境配置不变。完整备份 `/var/backups/glorynavy/before-v0.1.0-role-scope-20261019-20261018T171449Z.dump`（259260902 bytes）及同前缀 `.env`／`.meta.json` 留存，未执行本次备份的恢复演练。

线上 active/running/ready、NRestarts=0，首页／权限页／贷款页 200，匿名 scope-options／角色／目录／成员／贷款接口均 401；OpenResty 配置、HTML 和权限页／主包逐字节比对通过。提醒 enabled=false、发送及回执 0，发信 grant ready；QQ disabled、绑定 0。生产只读确认配置军团及联盟名称可用，未写入真实角色／授权或发送邮件。真实专项审批及合同交付继续按 ROLE-01 验收，不将隔离测试等同于真实业务验收。

回退时先恢复前端 `v0.1.0-welfare-officer-name-20261019`，再按需恢复后端 `v0.1.0-reminder-content-20261019`；新界面依赖 scope-options，不能单独先回退后端。角色范围仍使用原 ID 数组，无迁移或 Down，不用旧数据库覆盖新增业务记录。材料位于忽略目录 `.local/role-scope/`，远程发布源码 `/root/glorynavy-role-scope-release-20261019`。

## 2026-10-19 福利专员更名（仅前端）

前端 `v0.1.0-welfare-officer-name-20261019` 来自干净且已推送 main 提交 `be39d1acdc8439c4bca51c19107247f1d6098e8d`。入口和默认角色名称改为“福利专员”；旧默认名的单能力角色复用原 ID，在明确保存后更名，范围和成员授权不变。生产角色表只读核查为空，本轮没有写入角色或权限。

类型检查、生产构建、受影响 lint、5 项词表单测、中英文 1280/375/320px 的 9 项浏览器流程及截图检查通过。首次词表测试工作目录错误，改在 web 下运行后通过；GitNexus 索引已更新，提交前检测 LOW。干净 main 的 139 个文件与已测产物一致；发布包 SHA256、OpenResty 检查、公网页面 200、匿名权限／贷款接口 401、HTML 和权限页／主包逐字节校验通过。材料在忽略目录 `.local/welfare-officer/`。

仅切换边缘静态资源，后端保留 `v0.1.0-reminder-content-20261019`、Goose 98，无迁移或服务重启。可独立重新激活前端 `v0.1.0-reminder-events-20261018` 回退显示；保留数据库、角色授权和提醒配置，不恢复业务备份。

## 2026-10-19：业务提醒邮件正文

后端 `v0.1.0-reminder-content-20261019` 来自干净且已推送 main 的 `7c353d2a403bd7a575d5fe7a05ce338e4e45bed4`；前端继续使用 `v0.1.0-reminder-events-20261018`，没有前端源文件改动。6 个 Linux 二进制、发布来源元数据及 SHA256SUMS 在应用机从干净 main 构建；前端文件复用校验通过的当前发布包。Goose 98、River 不变，无环境配置变更。

通过标准 `activate-release.sh` 停服备份后切换。备份 `/var/backups/glorynavy/before-v0.1.0-reminder-content-20261019-20261018T161209Z.dump`（259356186 bytes）及同前缀 `.env`／`.meta.json` 保存原后端版本；本轮未做恢复演练。上线检查 active/running/ready、NRestarts=0、发布包校验、提醒页 200、匿名配置／历史 401 均通过。

推送设置保持 enabled=false、revision=2、events={approval_pending}，发件授权 ready 且含发信 scope；发送／回执均 0。QQ 仍 disabled、绑定 0，旧福利编号修复审计 12 条。所有测试使用独立 PostgreSQL 临时容器和随机 schema，结束后移除测试容器，未在生产创建测试单据或发送邮件。实际游戏内排版／收信待 REMINDER-01。

正文功能无迁移，可在关闭发送并处理未结束提醒任务后恢复 `v0.1.0-reminder-events-20261018`，保留 Goose 98 及已有回执；旧版将恢复汇总正文，不能回退到不识别多类型回执的更早版本。完整数据库恢复须同时保留备份环境与对应二进制，不执行 Down。


## 2026-10-18 多类型游戏邮件提醒

前后端 v0.1.0-reminder-events-20261018，来源为干净且推送的 main ab4fbce475a59f71bba005d97504761b09b4b02e。Goose 97→98，River 和环境配置不变。保留发件人、授权、revision=2 和 enabled=false，所选类型仍为原 approval_pending；迁移不发信或改变业务账务。

- 备份：/var/backups/glorynavy/before-v0.1.0-reminder-events-20261018-20261018T144641Z.dump（256725980 字节），同前缀 .env/.meta.json；停服后备份、激活时迁移。未做该新备份的恢复演练。
- 最终 10 项 reminder 测试含 race；新来源归属/状态与权限、EVE 代次/合同新鲜度、PAP 月末/未知证据、建筑燃料抖动、审批旧回执升级及新类型 Down 防护通过。宿主组合/元数据/CSRF 与相关 go vet 通过。测试数据库为同机独立 PostgreSQL 18.6 tmpfs 容器，未在生产库执行测试。
- 前端类型/构建/lint、9 项单测和中英文三种宽度 18 项浏览器流程通过；检查桌面与窄屏截图。干净 main 重建的 139 个文件与已测构建逐字节一致。后端 6 个程序与边缘发布清单哈希检查通过。
- 线上 active/running/ready、NRestarts=0，Goose 98；公网提醒页 200，配置 GET/PUT、历史 GET、重试 POST 和 purpose=reminder-mail 授权 POST 匿名均 401。QQ 两种 webhook 503、开关 false、绑定 0；邮件发送/回执 0，发件 grant ready 且 mail scope 保留，原福利旧号审计 12 条保留。
- 主包 index-BqE1P6o0.js SHA-256 d5f50d6a2ca9ff0bb1593e72f92d13cff7574e0f3c823ed201af39ede7263141；提醒包 page-x5uU3dpd.js SHA-256 d9ae5b167d9d10aeefe2e9386a4aa6afe550946e9cb2dfd8da4bd60c13a6dfed。公网 HTML/资源与发布文件相同，OpenResty 配置通过。
- 恢复：保留 Goose 98 与分类型回执；使用新类型后 Down 拒绝。回退旧发送程序前必须关闭发送、取消 reminder.scan.v1/reminder.send.v1 未结束任务；旧程序不能在新回执上执行跨类型清理。前一后端为 skills-queue-confirmation、前端为 feedback，必要时成对恢复程序并保持提醒关闭，不为程序回退直接覆盖业务数据库。

材料在本地 .local/reminder-events/；服务构建源码在 /root/glorynavy-reminder-events-release-20261018。当前尚未启用真实邮件，不把 API/隔离测试当作游戏客户端收信验收。


## 2026-10-18 军团技能要求待确认修正

后端 v0.1.0-skills-queue-confirmation-20261018，干净且推送 main 来源 9d2741de872667c22c01476ac5261592b3a98826；前端沿用 feedback，无新迁移，Goose 97。按官方 OpenAPI 的离线训练语义补齐已完成队列，不用两个接口抓取时间先后判定冲突。

- 激活前完整备份：/var/backups/glorynavy/before-v0.1.0-skills-queue-confirmation-20261018-20261018T055804Z.dump（251892293 字节）及同前缀 .env/.meta.json，未执行该新备份的恢复演练。环境配置未修改。
- 9 项技能测试、EVE 保留/撤权/解绑/归属与代次的 6 个子场景（race）、2 项宿主回归及 go vet 通过。覆盖队列先抓/后抓/同时抓、历史观测、未来完成时间、SP 合并、不提升生效等级、不降低已训练等级和成长资格同源检查。
- 独立发布源码编译的只读诊断使用正式技能检查及 EVE 观测服务，以 default_transaction_read_only 强制只读；4 个实际受影响角色 × 8 个方案共 32 结果，16 met/16 missing、unknown_requirements=0。未改原始快照、方案或福利记录，没有发放操作。
- active/running/ready、NRestarts=0、Goose 97、发布包 SHA256 通过；保留前轮 12 条单据编号别名。QQ/提醒关闭、QQ 绑定 0。公网 /skills 为 200；匿名 skills/context、plans/1/check、characters 以及结算接口均 401；QQ 回调 503。既有 HTML 和主包/技能资源与前端发布目录逐字节一致：主包 index-irX5B20i.js SHA256 0dae003bf747aaa7a957a04759499049a25594141a7222ccdb65fd9e52078df1，技能 page-DKTHLBk8.js SHA256 a49d793e2b37415a90992de314323a2958c756b6ddcc59e11993da1cfb91685f。

恢复可使用上一后端 remaining-settlement-ids 版本，保留 Goose 97 及福利旧号映射，不恢复旧数据库；回退会重新出现按抓取顺序误判的旧行为。前端不需回退。操作材料在忽略目录 .local/skills-pending/。


## 2026-10-18 其余福利结算编号

后端 v0.1.0-remaining-settlement-ids-20261018，来源干净且已推送 main 提交 37f3f0587bcca92954a171b5461c2d83445150b4；前端沿用 feedback，Goose 97。开发中的提醒扩展未包含在发布中，其未发布迁移顺延为 98。

- 激活前完整备份：/var/backups/glorynavy/before-v0.1.0-remaining-settlement-ids-20261018-20261018T053831Z.dump（251848608 字节，Goose 96）及同前缀 .env/.meta.json。本轮没有修改环境配置，也未对该新备份执行恢复演练。
- 北京时间 2026-10-18 13:39:40 开始停服修复事务。复核精确原编号、版本、状态、金额、账号与角色、30 分钟内成功合同同步、无批次/交付证据/有效合同/领取占用后，仅更新指定 12 条福利编号与 version。事务比较全量 123 条福利其余字段、61 条游戏合同占用、94 条福利占用、30 条批次明细、9 个批次、0 条兑换、2057 条币账及 10971 条福利审计保持不变。新增 12 条单据修复审计，不改写游戏合同标题或证据。
- 映射：#100→WF-20261005-000003；#101→WF-20261005-000004；#102→WF-20261005-000005；#103→WF-20261005-000006；#109→PVP-20261007-000007；#114→PVP-20261011-000008；#117→WF-20261011-000009；#119→WF-20261013-000010；#120→WF-20261013-000011；#121→WF-20261013-000012；#122→WF-20261013-000013；#123→WF-20261017-000014。原号完整保存在 welfare_case_reference_repairs。
- #9/#12 保留原号，对应 236787534/236928495；#13/#14 保留短号，对应 237159429/237159344，四个批次仍待领取；另 5 个已完成批次保持原状。修复的 3 条 approved 与 9 条 submitted 状态不变；未完成且未入批次的福利已无 UUID 长号。
- 相关隔离数据库回归（业务含 race）、go vet、脚本隔离演练/重复执行拒绝通过；既有取消回退测试补充恢复最新 schema 的步骤后复验通过。线上 active/running/ready、NRestarts=0、Goose 97、发布包 SHA256 通过。公网 /feedback、/courier、/approvals 为 200，匿名结算和业务接口 401；QQ 回调 503、QQ 绑定 0、提醒关闭。前端仍为 feedback，HTML 与全部受检资源逐字节相同，主包 index-irX5B20i.js SHA256 为 0dae003bf747aaa7a957a04759499049a25594141a7222ccdb65fd9e52078df1。未发送通知或创建游戏合同。

已有 12 条修复审计，Goose 97 Down 会拒绝。恢复须保留单据新旧号映射、批次旧号映射、共享序列及相应兼容程序；不能直接切回 feedback 后端或用旧备份覆盖修复后业务。金额/物品/收件人/发放者权限等仍按原规则核验，新旧号均有有效合同时阻止自动发放。操作与验证材料在忽略目录 .local/remaining-settlements/。

## 2026-10-18 意见反馈

前后端 v0.1.0-feedback-20261018 来自干净且推送的 main 提交 3520792f0aa903e60bf02b607887dfc88f94a254。独立 main 克隆构建的前端 139 个文件与浏览器验证产物逐字节相同。新增 feedback 模块与 Goose 96；未包含主工作区开发中的提醒扩展，其迁移文件已顺延至未发布的 97。

- 配置前备份：/var/backups/glorynavy/before-feedback-config-20261017T185127Z.env。仅向既有 MODULES 追加 feedback，其余配置保留。
- 激活前完整备份：/var/backups/glorynavy/before-v0.1.0-feedback-20261018-20261017T185131Z.dump（252419372 字节），同前缀 .env/.meta.json 保留来源后端版本；激活备份的环境已包含 feedback。
- 上线验证：Goose 96、active/running/ready、NRestarts=0，feedback_tickets/messages 初始均 0、活跃账号触发器 2。公网 /feedback、/courier、/approvals 为 200；匿名反馈列表/全量范围/详情/提交/回复、模块目录均 401。QQ 回调仍 503，OneBot 关闭/绑定 0，提醒关闭。没有创建生产测试单据或发送通知。
- 公网 HTML、主包与反馈/快递/审批资产和发布目录逐字节一致。主包 index-irX5B20i.js SHA256 为 0dae003bf747aaa7a957a04759499049a25594141a7222ccdb65fd9e52078df1。
- 验证：隔离 PostgreSQL 4 项反馈业务测试（race）、2 项宿主反馈/停用合并测试、2 项底座/元数据回归、go vet；前端类型/构建/lint、9 项注册/语言单测、中英文 1280/375/320px 共 18 项浏览器流程及截图检查通过。sqlc 全量重生成一致，既有 structures 生成文件 SQL 条件/参数不变。材料在忽略目录 .local/feedback/。

恢复优先保留当前 schema 与 feedback 合并参与方。已有反馈时禁止 Down，也不能恢复遗漏该参与方的旧二进制。仅当确认新表无任何记录且需要恢复到原版本时，才可停服恢复配套备份/原配置与旧前后端；恢复配置时不能把带 feedback 的新 MODULES 直接交给不认识该模块的旧程序。保留 Goose 95 合同别名兼容边界。功能关闭只移除 MODULES 的 feedback，已有数据和合并参与方仍保留。

## 2026-10-18 快递服务与合同复制区

后端 v0.1.0-courier-20261018 源码为干净且推送的 main 提交 6464e8dc1802a8fb2d0f21280ac11bffbaa157da；最终前端 v0.1.0-courier-copy-20261018 来自干净且推送的 main 提交 fdda7705a7e28f527a63307fbd13a45debf334ea。前端在独立干净 main 克隆构建，未包含开发中的提醒扩展；后端仍为 Goose 95、active/running/ready、NRestarts=0。

- 配置变更前原始环境备份：/var/backups/glorynavy/before-courier-config-20261017T181932Z.env。只追加 MODULES 的 courier 与专用 COURIER_HTTP_PROXY；使用已存在的受信任出站代理，未改变 ESI/SSO 的网络配置。
- 完整业务备份：/var/backups/glorynavy/before-v0.1.0-courier-20261018-20261017T181936Z.dump（252379394 字节）及同前缀 .env/.meta.json。该常规激活备份的环境已含 courier；如回退到不认识 courier 的旧后端，需要恢复上述配置变更前环境或移除 courier 模块项，不能直接搭配新环境启动旧程序。数据库没有新迁移；必须保留 Goose 95 的两条编号别名与认识别名的程序。
- 公网 /courier、/approvals 为 200；匿名快递读写、结算详情、模块目录均 401；QQ OneBot 回调继续 503，绑定 0、提醒关闭。最终 HTML、主包、快递/审批 JS 与 CSS 和发布目录逐字节一致；主包 index-Dc638JKK.js SHA256 为 ad82c549887fce8440dc9a8d6c75baa211a2b03681c5cad064146528cb5b13e0。
- 发布源码编译的只读诊断在实际 EnvironmentFile 下读取 254 条有效路线，Tritanium 3 返回体积 0.03 m³、出售价 11.7 ISK（2026-10-18 02:20 北京时间报价，非固定价）。没有创建游戏合同或发送通知。#14 在部署时由后台正常同步到新游戏合同 237159344，仍 pending；最终 UI 将游戏合同 ID 改为摘要文本，复制区固定三项。
- 5 项服务测试（race）、4 项隔离库宿主/估价/元数据测试、go vet、前端类型/构建/lint、4 项计算单测以及中英文三种宽度的 24 项快递和最终 24 项批次浏览器测试通过。参考仓库未选择开源许可证，本项目独立实现并记录来源；不复制其代码或资源。材料保留于忽略目录 .local/courier/。

前端可独立回退，但会恢复旧复制框；无需降级数据库。停止快递功能可从 MODULES 移除 courier 并重启，模块没有持久账号数据或待处理任务。游戏内创建、接取与实际运输由玩家办理，不将报价联调当作运输完成证明。


## 2026-10-18 物品名称复制标记

后端 v0.1.0-market-name-marker-20261018 来自干净且已推送的 main 提交 c64443935ae66750af761bf451d859f1b2583048；前端沿用 contract-batch-flow 版本。Goose 95、active/running/ready、NRestarts=0，发布包哈希通过。完整备份 /var/backups/glorynavy/before-v0.1.0-market-name-marker-20261018-20261017T175423Z.dump（252354099 字节）及同前缀 .env/.meta.json。保留 Goose 95 的两条编号别名；回退只能使用同样理解别名的程序。10 项 market、4 项 EVE 测试及 go vet 已在隔离环境通过，真实 SDE 与公开报价诊断确认用户原文数量 3+2 完整识别。



## 2026-10-18 批次编号与合同进度

前后端 `v0.1.0-contract-batch-flow-20261018` 来自干净且已推送的 `main` 提交 `44c69113bb021a0f85ff77adc9894a50cc471cf1`，Goose 95、active/running/ready、NRestarts=0。发布不包含开发中的其他提醒类型；它的未发布迁移顺延为 96。

- 发布前完整备份 `/var/backups/glorynavy/before-v0.1.0-contract-batch-flow-20261018-20261017T173625Z.dump`（252354836 字节）及同名前缀 `.env`、`.meta.json`，备份 schema 为 Goose 94。备份停服窗口首次公网探测返回 502，服务就绪后全部重查通过。
- 用户明确授权的修复在 2026-10-18 01:37:35（北京时间）停服事务内执行：重查主角色、30 分钟内成功合同同步、无有效/完成同号合同、精确金额和来源清单；#13→`BATCH-20261013-000001`，#14→`BATCH-20261013-000002`。两条旧号别名、操作者、原因、时间保存在 Goose 95 表。事务比较全量 123 条福利、61 条合同占用和 30 条批次明细均未变化，其余批次业务字段不变；#9/#12 仅提前安排自动检查。
- 自动后台核验后 #9/#12 的游戏合同为 236787534/236928495，均“等待领取合同”；#13“等待合同”，#14 原游戏合同 237156587“合同已删除”。四个批次完成数为 0，未创建游戏合同或模拟付款。全部福利与其余 7 个批次历史编号摘要保持不变。
- 公网 `/approvals` 200，匿名结算读写和审批 API 401，两种 QQ webhook 503。HTML/主包/审批包与发布包逐字节相同；主包 `index-CzRrZp0g.js` SHA256 `495b07808ec67b5aedd0f72218753d8398f6b40acd749a808c68743ce595842d`，审批包 `page-DwkuEnlU.js` SHA256 `511c96ed020b4377210de3d6c73b9834cea83b80482b18dd4539bcc79ee354b7`。QQ 仍关闭、NapCat exited、绑定/有效确认 0；提醒关闭、邮件 0。
- 25 项隔离数据库测试和相关 go vet、前端类型/构建/lint、11 项单测及中英文三种宽度的 36 项浏览器流程通过。Windows 本地 Go 下载失败后采用服务器隔离 PostgreSQL 测试，未在生产库跑测试。材料在忽略目录 `.local/batch-flow/`。

已有两条编号修复审计，Goose 95 Down 会拒绝。恢复必须保留新旧号映射、当前序列及别名匹配程序；不能直接切回 Goose 94 程序或删除审计。恢复完整旧备份会丢弃之后的业务与编号修复，不能用于普通界面回退。若出现新旧号双合同，必须核对游戏证据后只保留唯一有效交付，不通过改完成数绕过核验。


## 2026-10-18 短合同结算编号

前后端发布 `v0.1.0-short-contract-reference-20261018`，来源干净且已推送的 `main` 提交 `e0d8f78d584d77f326c9cda470c1c9d74d1069e4`。应用与边缘 current 均已切换，Goose 94、active/running/ready、NRestarts=0，发布清单 SHA256 和 OpenResty 配置检查通过。前端短编号复制与历史 50 字符兼容、窄屏换行同批发布，未包含在开发中的其他提醒类型。

- 发布前备份：`/var/backups/glorynavy/before-v0.1.0-short-contract-reference-20261018-20261017T165705Z.dump`（252283571 字节）及同名前缀 `.env`、`.meta.json`。备份中包含 Goose 93；恢复数据库会回退备份之后的业务数据，不能只为界面回退而恢复旧库。
- 123 条福利与 9 个合并批次编号摘要在迁移前后一致（既有兑换 0 条）；新函数引用非循环 sequence，当前 last_value=1/is_called=false。没有为验证创建生产业务记录或消耗编号。
- 公网 `/approvals` 200，匿名结算 GET/POST、审批列表均 401，QQ 两种 webhook 均 503。HTML 与包一致；主包 `index-DH9FAL5N.js` SHA256 `de81e0f6aeee2cca15870448cafdbf7f1a5b4f21d0f31a386186ef829d66cc92`，审批资源 `page-n8D6KT2J.js` SHA256 `c4e563a158cecbfa48f34e5e4fe258b64789be13fcac8de11400277ead8c70a1`，逐字节核验。
- QQ 仍 disabled/revision=6、NapCat exited，绑定/确认 0；邮件提醒仍 disabled/revision=2、发送记录 0，未修改通知开关或发送消息。
- 恢复须保留 Goose 94 和编号序列；保存短编号后 Down 明确拒绝，不重置序列、不改写旧单。原交付校验可读取历史与新编号，游戏合同 ID 保持 ESI 数值。真实新合同完成验收仍见 CONTRACT-02。

本轮 23 项隔离 PostgreSQL 回归与相关 go vet、前端类型/构建/lint、2 项标题单测及中英文 1280/375/320px 的 12 项浏览器流程通过；截图检查通过。脚本和浏览器证据在忽略目录 `.local/short-contract/`。

## 2026-10-17 暂停 QQ 机器人

用户要求暂时禁用。通过当前部署源码的正式 `community.SaveOneBotSettings` 服务，以原 revision、API 地址、群配置及保留的加密凭据保存 `enabled=false`，新 revision=6；未直接写业务 SQL。停止独立 `glorynavy-napcat` 容器，确认 exited/running=false，保留 `unless-stopped` 策略及持久目录。

复核数据库全局开关关闭、接收群仍 1 个、未结束 `community.approval-group.v1` 任务为 0、发送批次仍 1 条 sent；新 OneBot 和旧官方 webhook 均返回 503。网站 active/running/ready、NRestarts=0，前后端版本和 Goose 不变。配置、绑定及去重历史保留；没有发送测试消息。恢复须用户明确要求后重新启动 NapCat、确认 QQ 在线再启用网站配置，不能仅开启群提醒复选框。运维脚本在忽略目录 `.local/qq-disable/`。

用户进一步要求退出 QQ 账号登录：在容器已停止时清空 `webui.json` 的 `autoLoginAccount`，核对没有环境变量自动登录账号或密码回退；只清除该账号登录凭据文件及 `nt_qq/global/nt_db/login.db`、WAL、SHM，共 4 个文件。个人聊天数据库、设备信息、网站绑定和 OneBot 配置保留。关闭自动登录后短暂启动核验，WebUI 返回 isLogin=false、coreReady=false、loginPhase=waiting_qrcode，快速登录账号列表为空、自动登录账号为空；随后容器再次停止并确认 exited/running=false。未扫码、未启用网站机器人、未发送消息；以后恢复需本人重新扫码确认。此次为清除服务器登录状态，不代表删除手机 QQ 的历史设备列表。脚本为同目录 `logout.py`。

用户再要求移除已绑定账号：事前检查为 1 个账号，OneBot/旧官方绑定各 1 条、群绑定 0、有效 QQ 确认 1。现有接口没有解绑端点，本次执行受范围断言保护的一次性事务：锁定停用配置、有效账号与资料行，删除两种绑定，撤销 QQ 确认、使目标账号旧验证码失效、停用 9 条活跃提醒回执；递增 QQ/资料版本并写入资料变更事件。事务断言手填 QQ、KOOK 和 KOOK 版本保持不变；历史确认与发送批次保留。提交后 OneBot/官方/群绑定均 0、有效 QQ 确认 0，机器人仍停用 revision=6。事前另有 1 条未使用验证码属于未绑定账号，本次不扩大到该账号，原到期和全局停用约束继续生效。未清除 EVE 角色绑定或本站权限；以后需重新验证码核验 QQ。脚本为同目录 `unbind.sql`、`unbind.py`。



## 2026-10-17 业务专员配置（仅前端）

前端 `v0.1.0-officer-roles-20261017` 来自干净且已推送的 `main` 提交 `6bdafbf3b29e45e121f426e85d2735df4456cf25`。补损、福利发放、贷款专员通过既有权限角色和成员授权配置；游戏合同处理角色单独配置。用户取消 PAP 结算专员，不增加其权限。没有代用户创建角色或授予生产成员权限。

- 生产分支守卫、发布清单 SHA256、边缘 current 与 OpenResty 配置均通过。后端保持 `v0.1.0-qq-approval-notify-r2-20261017`，active/running/ready、NRestarts=0，Goose 92；本次没有后端部署、数据库迁移或 QQ 服务重启。
- `/`、`/access`、`/loans/operations` 均 200；匿名 roles/catalog/members、loan context/policy 均 401。HTML SHA256 `d1747bd9fa8cb01bff21fc1f0b3980658e3fd1b839ec3377bf4f1e38d4cc952d` 与包一致。
- 公网管理资源 `management-page-BOiWrP85.js` SHA256 `70b3d1a897958e8e388f273f9d5cb86fcaa67717b37ffc2be3083b46f51022f6`；贷款资源 `page-Bow6YhC7.js` SHA256 `943d92b38d773ad509920dc5dec6ed2d9700f097ce32c046878c0f1afca4af71`，逐字节核对一致。
- 前端类型/构建、受影响文件 lint、5 项词表单测、27 项浏览器流程及最终翻译后的 2 项窄屏复验通过；覆盖权限拒绝、角色保存、授权/撤销、跨语言复用和过滤保留，1280/375/320px 截图已检查。全量 lint 的 5 项既有警告不在本次修改范围。后端配置军团范围和补损权限两个已有单测复验通过。
- GitNexus 索引落后，刷新停滞后停止；既有页面 impact 为 LOW，部分新组件/回调和词表为 UNKNOWN，人工复核。提交扫描为 17 文件、21 个旧索引符号、2 条贷款详情翻译流程，MEDIUM。证据与发布工具位于忽略目录 `.local/officer-config/`。
- 恢复可重新激活保留的前端 `v0.1.0-qq-approval-notify-r2-20261017`；角色/成员数据接口未变，撤回前端不会撤销已保存的权限角色。后端和数据库不随前端回退。

## 2026-10-17 待审批提醒成员查询修正 r2

前后端 `v0.1.0-qq-approval-notify-r2-20261017` 来自干净且已推送的 `main` 提交 `6d6faf90f285de678b857061efacd23c2c217f9a`，Goose 92 不变。修正仅为通知成员查询：全表缓存用于候选，空结果报不可用，发送前逐个强制查询并核对群/成员身份。独立 PostgreSQL 通知专项含扫描后离群共 4 个顶层测试及 9 个边界子场景通过；相关 vet、app/server 编译通过。

- 应用 active/running/ready、NRestarts=0，前后端 current 和 SHA256 清单、OpenResty 配置、公网页面、匿名管理/审批 401 及新回调缺签名 401 均通过；前端代码未变，HTML/管理资源哈希与下方首版一致。
- 备份 `/var/backups/glorynavy/before-v0.1.0-qq-approval-notify-r2-20261017-20261017T140304Z.{dump,env,meta.json}` 均 0600，dump 253226186 字节；未做本轮恢复演练，首版发布包保留。
- 原群提醒临时关闭（revision=4）后，在 r2 上经正式服务恢复同一已配置群（revision=5）；未新增目标或更换机器人。独立 NapCat 重启后登录态失效、快速登录名单为空，已配置预期账号的自动登录，但未以再次重启验证。后续用户重新扫码并确认后，22:21:35 记录扫码，22:21:42 QQ 返回 serverErrorCode=168，提示账号安全风险及部分功能受限，要求在最新版手机 QQ 恢复账号使用；容器 running、restart=0、oom=false。当时 WebUI 仍显示 qrcode_scanned、isLogin/coreReady=false，不能把该阶段当作用户尚未确认；后续仅按用户要求重新发起正常扫码登录，未绕过安全验证。本站服务与 QQ 登录状态分开：网站 ready 不代表机器人在线或通知送达。
- 登录恢复复验：用户再次扫码确认后 isLogin/coreReady/online/good 均为 true，实际群匹配既有配置，缓存成员 22 人；当前绑定管理员的单成员查询（no_cache=false/true）均返回身份匹配。北京时间 22:28:05 正常 River 群任务完成，发送批次 sent=1、接收人 1、待办与回执各 9；22:28:57 后续群扫描完成，批次仍为 1。配置 enabled=true、revision=5、提醒群 1 个保持不变。未创建测试审批或手动发送业务提醒；用户随后明确确认“收到，@显示正常”，该真实审批提醒的发送、客户端接收和指定成员 @ 验收通过；其他业务通知仍按 COM-02 跟踪。

## 2026-10-17 待审批 QQ @ 提醒

前后端已发布 `v0.1.0-qq-approval-notify-20261017`，来源为干净且已推送的 `main` 提交 `f9a05ee9d609d9465ecb0daaf31435e193f2199b`。Goose 92 新增 community 通知回执/发送批次及接收群提醒开关；原审批来源继续保有业务状态和权限。配置及回退见 [OneBot 指南](integrations/qq-onebot.zh-CN.md)。

- 备份 `/var/backups/glorynavy/before-v0.1.0-qq-approval-notify-20261017-20261017T134709Z.{dump,env,meta.json}` 均为 0600，dump 253181293 字节；本轮未恢复该新备份。旧版 `v0.1.0-qq-onebot-20261017` 保留。回退前须停用提醒并取消新任务，保留去重回执，不能盲目 Down。
- 两机 current、SHA256 清单、OpenResty 配置及公网页面通过；后端 active/running/ready、NRestarts=0、Goose 92。匿名管理 GET/PUT/状态/测试与审批列表 401，新回调无签名 401。
- 公网 HTML SHA256 `0ff3288df9a400e0b86140684ac0adf12e480dec31a1d46703cf0ad1f16247cf`；管理资源 `qq-admin-page-6Zzw-UT1.js` SHA256 `58978291b3ef987e837fea7bc40e1b00da27948496064bbf9bb2a0f300baa272`。
- 使用正式 community 配置服务开启现有一个真实群的审批提醒，revision=3；先核对当前管理员和真实加入群。机器人在线，群内当前有效 OneBot 绑定 1 个。未造生产审批，真实触发和客户端效果见项目状态与 COM-02。
- 隔离 PostgreSQL 的 community/approval/app/jobs 回归、相关 go vet/server 编译、sqlc 一致性、前端类型/lint/构建、5 项词表与 2 项中英文页面流程通过，1280/375/320px 截图已检查。日志及复验材料在忽略目录 `.local/approval-notify/`。

## NapCatQQ / OneBot 11（2026-10-17）

前后端已发布 `v0.1.0-qq-onebot-20261017`，来自干净且已推送的 `main` 提交 `9fb785c737a6a8ea1c3374a89adb01d0df4b14cf`。Goose 91 新增 community 私有配置、群与数字 QQ 绑定；旧官方历史记录保留，首次保存 OneBot 配置才切换，存在配置或绑定后禁止直接 Down。详细配置与恢复边界见 [NapCat 指南](integrations/qq-onebot.zh-CN.md)。

- 发布前备份 `/var/backups/glorynavy/before-v0.1.0-qq-onebot-20261017-20261017T074424Z.{dump,env,meta.json}` 均为 0600，dump 253716338 字节；本轮未恢复该新备份。旧版 `v0.1.0-qq-group-code-20261017` 保留。
- 两端发布清单与 current 已核对；应用 active/running/ready、NRestarts=0，Goose 91，OpenResty 配置检查通过。`/`、`/account`、`/community` 均 200；新管理 settings GET/PUT、status GET、test POST 匿名均 401。初始未配置时 OneBot webhook 返回 503；实际启用后缺签名新回调返回 401、旧官方回调返回 503。
- 公网账户 HTML SHA256 `0d9f2adfd01fc8a27d4653af136a464bfac3d47c43689b286b82bc66ee327173` 与包一致；`qq-admin-page-9YmnFfUe.js` SHA256 `5e65232e1c7bc65ef2a783f736a42af79dbd323d701119d8c2f51a6e05de739a` 与包一致。
- 独立 NapCat 容器固定 v4.18.37 镜像摘要，见[部署示例](../deploy/compose.napcat.example.yaml)。WebUI 和 OneBot HTTP 仅绑定宿主 loopback，持久目录和凭据为私有权限，日志限制 3×10 MiB；不公开登录二维码或凭据。现已完成登录，isLogin/coreReady/online/good 均为 true；经正式 community 服务保存加密配置并启用，revision=2，实际加入并允许的群为 1 个，官方传输已停用。
- 隔离 PostgreSQL 的 24 项 community 测试、community/app go vet 与 server 编译通过。宿主全量回归初次仅旧清理回退断言受新增迁移影响，明确 DownTo(89) 后，清理、社区门禁和新当前管理员/撤权/CSRF 共 3 项复验通过。Windows 本地 Go 依赖下载超时不作为通过证据；隔离测试容器已移除。
- 前端类型/lint/生产构建、5 项词表单测、2 项中英文新管理流程及 4 项原绑定流程通过，1280/375/320px 截图已检查；社区 sqlc 生成一致，OpenAPI YAML 及受影响文档链接检查通过。日志、截图、发布和验证脚本位于 `.local/qq-napcat/`。
- GitNexus 旧索引对共享初始化传播报 CRITICAL，提交扫描 44 文件、39 个已索引符号、22 条流程；已提前提示并按准确文件和调用点复核，宿主初始化仅增加账号锁注入一行。新增符号未被旧索引覆盖，不把索引结果当完整范围证明。
- 登录诊断历史：北京时间 15:47:19 记录扫码，15:47:20 记录 ErrType=1 / ErrCode=5；WebUI 仍为 qrcode_scanned 且 loginError 为空，不能解释为用户未确认。原 PNG 与当时 WebUI QR URL 一致，容器无重启/OOM；用户随后明确手机提示过期。刷新当前 WebUI 链接，直接生成并反向解码核对二维码后，20:30:52 收到新扫码并完成登录，未降级 NapCat/QQ。脱敏日志在 `.local/qq-napcat/login-diagnostics.log`；上游[同码报告 #2063](https://github.com/NapNeko/NapCatQQ/issues/2063)仅作为诊断线索，不作为此次根因证明。
- 真实绑定：用户加入群后，运维以正式 SaveOneBotSettings 服务校验和保存该实际群；用户自行在网站生成验证码并免 @ 发群，20:35:16 成功写入数字 QQ 绑定与当前 onebot 确认，绑定/事件计数各 1，未人工写绑定或代用户生成验证码。
- 主动 @：当前管理员与当前 QQ 确认复核后，调用正式 TestOneBotMention 向唯一已配置测试群发送一次固定“本站测试提醒”，20:42:26 返回成功；没有自动重试发送，客户端显示待用户确认，记录 `.local/qq-napcat/mention-test.log`。临时工具首次编译因文件传输未完成而失败，未执行发送；传输完成后编译并发送成功。
- 真实账号改号失效、异常/重放恢复及进程重启后的登录保持仍待验收；业务通知与自动入群尚未实现，按 COM-01/02 验收。

## QQ 群消息直接发码绑定（2026-10-17）

前后端已发布 `v0.1.0-qq-group-code-20261017`，来源为干净且已推送的 `main` 提交 `85dcfed1d8ed44d5b20384c2316af2525574fae9`，应用/边缘分支守卫及 SHA256 清单校验通过。个人账户只显示、复制 8 位验证码，群 `GROUP_MESSAGE_CREATE` 通过发送人的 `author.member_openid` 确认；普通聊天、机器人和缺少身份标识的消息静默处理。旧私聊/@ 事件兼容，无新迁移，Goose 90。

- 发布前备份 `/var/backups/glorynavy/before-v0.1.0-qq-group-code-20261017-20261017T055716Z.{dump,env,meta.json}` 均为 0600，dump 253593064 字节；本轮未恢复该新备份。旧版 `v0.1.0-qq-binding-20261017` 保留。
- 两端 current 已切换；应用 active/running/ready、NRestarts=0，OpenResty 配置检查通过。`/`、`/account`、`/community` 均 200；匿名 POST challenge 和缺签名回调均 401。
- 公网账户 HTML SHA256 `3da9e753a241680ea29fc518f98608372b7f90e548cca78da1c535b25c1c0861` 与包一致；`account-page-Bdk87z2x.js`、`index-9yEFDtvL.js` 公网字节哈希一致。
- 隔离 PostgreSQL 的 17 项 community 测试（52.102s）、community go vet、server 编译通过，测试容器已移除；前端类型/lint/构建、7 项 API/词表单测和 4 项中英文浏览器流程（37.8s）通过。1280/375/320px 截图已检查，生成仍待确认、剪贴板仅含验证码，过期和成功收起正常。日志、截图与发布脚本在 `.local/qq-group-code/`。
- GitNexus 索引刷新两次停滞后停止，旧索引对 QQBinding 未识别、回调/消息结构 impact 为 UNKNOWN；已补查 ProfileCard 与注册路由的实际引用。提交前 detect_changes 返回 14 文件、5 条 QQ 回调流程、MEDIUM，陈旧索引不作为完整范围证明。
- 实际联调边界：此前真实群 @ 收发已确认；本轮只做隔离签名回调与模拟前端流程，未代用户发送验证码。用户开通并订阅 `GROUP_MESSAGE_CREATE` 后仍需验证真实免 @ 绑定；群入群审批和后续通知未纳入本次发布。

## QQ 绑定码入口与一次性消费（2026-10-17）

前后端已发布 `v0.1.0-qq-binding-20261017`，来源为干净且已推送的 `main` 提交 `5bb3dede551883b3d7913b9c3165228fb0d62122`，生产分支守卫通过。个人账户 QQ 资料下新增生成/复制绑定指令、到期与确认反馈；后端绑定事务补上一次性码消费，无新增迁移，Goose 90。

- 发布前备份：应用机 `/var/backups/glorynavy/before-v0.1.0-qq-binding-20261017-20261017T053733Z.{dump,env,meta.json}`，权限 0600，dump 253556752 字节。原版 `v0.1.0-storage-retention-20261017` 保留。无迁移变更不等于应回退绑定码消费修复，回退需重新评估未消费码的重用风险。
- 应用及边缘发布清单 SHA256 校验通过，双方 current 均切换；应用 active/running/ready、NRestarts=0，OpenResty 配置检查通过。`/`、`/account`、`/community` 均 200；匿名 POST 绑定码接口 401，缺签名 QQ 回调 401。
- 公网账户入口 HTML SHA256 `f5dd84b2bef1bd8104472a2af47ad1ace0646ff26fe06f0173e38cae7e33b7d8` 与发布包一致；含绑定功能的 `account-page-I-SIdwF0.js`、`index-CRHerTrZ.js` 公网内容逐字节哈希一致。
- 14 项 community 测试在独立临时 PostgreSQL 通过，含重复消费/并发/失败回滚/过期与资料变化失效；测试容器已停止删除。community go vet、server 编译、前端构建、相关 lint、7 项 API/词表单测及 4 项中英文浏览器绑定流程通过；最终 1280/375/320px 排版与复制操作验证通过，截图及日志位于 `.local/qq-diagnostics/`。
- 本地 Go 首次验证因依赖下载超时未完成，随后在应用机独立源码目录/测试数据库完成；初次浏览器执行因预览未启动失败，构建并启动预览后最终 4 项通过。准备边缘包时一个维护脚本文件名错误导致打包中止，修正后重新生成并验证清单，随后才激活；不是生产功能失败。
- 真实平台边界：凭据、机器人身份、公网模拟握手与验签已验证，用户已确认真实群 @ 消息能获得回复；本次未代用户生成或提交绑定码，真实绑定待用户完成。群成员加入订阅项在用户后台不可见，自动入群审批仍待核实平台能力；主动通知仅登记为后续需求。

## v0.1.0-storage-retention-20261017 — 2026-10-17

前后端来自干净且已推送的 `main` 提交 `80ff71be674a8b2a827786c856b3e1edb335f4cf`，生产分支守卫通过。备份与发布包新增校验后轮换、恢复版本保护、部署互斥锁及每周定时器；同步执行历史按成功 7 天、暂缓/分页进度 3 天、其他完成记录 30 天保留，未结束记录保留。Goose 90，无新迁移；维护命令按每批 500 条补清。操作边界和停用方法见[存储指南](storage-retention.md)。

10 项 Linux 文件轮换测试、5 项独立 PostgreSQL 保留期/批次/续跑/索引回退及 184 万行查询计划专项通过；server/维护命令编译、相关 go vet、sqlc 1.31.1 生成一致性和前端构建通过。索引测试只执行自身 Goose 46 上下迁移，避免无关 Goose 73 Down 的既有解析问题，未修改生产迁移。GitNexus 重建索引后，提交检查为 19 文件、51 符号、0 条索引调用链、LOW，无 partial/error，实际范围已复核。本次没有页面行为变化，未重复钱包浏览器业务验收。

发布前备份 `/var/backups/glorynavy/before-v0.1.0-storage-retention-20261017-20261017T030126Z.dump` 为 455847027 字节，SHA256 `d56f28d277523a1d53a6e61b07b825646d4766721b615b6716ea321a4487704b`；dump、匹配 env 和原后端版本 meta 均为 0600。完整恢复至本轮隔离 PostgreSQL，通过 Goose 90 及钱包 124800、币流水 2047、贷款 1 的计数核对，随后移除本轮测试容器和专属卷。所有 13 份保留的完整备份均通过完整归档读取，相关保留发布包通过 SHA256 校验。

首次轮换删除应用旧发布 166 个、边缘旧发布 173 个、完整旧备份 190 组；保留 19 份 dump（13 份完整、6 份历史不完整），当前/恢复版本及未知布局均保留。以下为发布并生成新备份之后、执行轮换前后的 `du -s -B1`，合计回收 77829398528 字节（72.48 GiB）；不计测试库移除或数据库内部回收。

| 目录 | 清理前字节 | 清理后字节 | 回收 GiB |
| --- | ---: | ---: | ---: |
| 应用备份 | 62502584320 | 6213242880 | 52.42 |
| 应用发布 | 14721261568 | 3420205056 | 10.52 |
| 边缘发布 | 11084509184 | 845508608 | 9.54 |

完成清单保存在应用 `/var/log/glorynavy-storage-retention/20261017T030720.144119Z-app.json`、边缘 `20261017T030721.203049Z-edge.json`，均含 completed=true，清理后两机再次预览均无候选。两机定时器 enabled，下次北京时间 2026-10-19 应用 04:26:58、边缘 04:22:32；每周基准 04:20，随机延迟最多 15 分钟。最终根分区占用应用 25%、边缘 39%。

同步历史从 11395479 条降至 2898644 条（分别取 03:06:45 和 03:27:33 UTC 快照，期间后台仍写入和清理）。补清工具累计删除 8490784 条，以首个快照时间减去各保留期复查，原积压剩余 0；即时余量 111 条新到期记录由正常调度处理。前两次补清与后台取同批候选而提前返回空批，随后有界续跑；续跑第 22 轮已无大批积压，其后只有新到期的少量记录，达到 60 轮保护上限后退出 1，未将该退出码冒充全量完成证据。完成判断采用固定起始水位复查，保留完整执行日志。

`VACUUM (ANALYZE, TRUNCATE FALSE) eve_sync_runs` 已成功，未执行 VACUUM FULL 或重建全库。最终数据库 5098911423 字节，同步历史表及索引 3997974528 字节；删除行的内部空间可复用，不能算作已释放给文件系统的空间。

应用 current 与边缘 current 均为新版本；文件轮换后应用 active/running/ready、NRestarts=0，首页/工作台/钱包 200、匿名钱包上下文 401、发布哈希与 OpenResty 配置检查通过。公网 HTML SHA256 `7f11e1ba19845d54e7069d2ae592ff805491bd75c0a1cd0831e3d81701b5c51d`。钱包、币账和贷款数量仍与恢复备份一致；没有清理生产业务数据或不明 Docker 卷。日志和清单副本位于本地 `.local/storage-retention/`。

## v0.1.0-wallet-game-labels-20261017 — 2026-10-17

仅发布静态前端，来源为干净且已推送的 `main` 提交 `a01806f07b6854f3d2c76a3b46f35879c8119541`，生产分支守卫通过。税金行按收入来源使用官方 SDE 游戏名称，移除税前小字、计算说明及额外税前/到账字段；金额、分行、筛选和原始数据口径不变。应用继续运行 `v0.1.0-wallet-tax-rows-20261017`，未执行迁移或重启应用，Goose 90 沿用。

前端类型检查、lint、构建、17 项钱包/语言单元测试及桌面/375px/320px 中英文 6 项浏览器检查通过，桌面列表及中英文手机详情截图已查看。修改前 History 影响检查为 LOW，直接调用者 Account、上游 Workspace/WalletPage。刷新索引后的 detect_changes 对 10 个文件返回 794 条流程/CRITICAL，新增函数 ID 为空，重新 impact 也出现空类型和含 NUL 的路径；该结果不能作为有效调用范围。实际 diff 及引用检查确认仅钱包名称辅助函数、History 展示、钱包 CSS、英文词条、对应浏览器断言及文档变更；新增函数只用于列表和详情两处，删除的词条无剩余引用。

边缘 current 为新版本，发布包哈希、公网 HTML 与 OpenResty 配置检查通过；首页/工作台/钱包均 200，匿名钱包上下文、汇总与三种方向筛选均 401。公网 HTML SHA256 `7f11e1ba19845d54e7069d2ae592ff805491bd75c0a1cd0831e3d81701b5c51d`。应用 active/running/ready、NRestarts=0。旧前端发布保留，可独立回退至 `v0.1.0-wallet-tax-rows-20261017`。

日志与截图在本地 `.local/wallet-game-labels/`；本地预览已关闭。未创建生产测试流水或使用成员会话，浏览器业务检查使用隔离响应夹具。

## v0.1.0-wallet-tax-rows-20261017 — 2026-10-17

前后端来自干净且已推送的 `main` 提交 `c0d3950b8dfae5cccd6c7518219554852a55e4f0`，生产分支守卫通过。个人收入与军团税独立成行，税前金额与税款精确相抵为原始到账；按展示方向筛选，支持实际到账 0 的全额代扣。保留来源编号、最终余额与原始汇总，无迁移，Goose 90。

隔离 PostgreSQL 的 EVE 钱包、wallet 模块与钱包/军团范围宿主回归、server 编译及相关 go vet 通过。前端类型/lint/构建、17 项单元测试、桌面/375px/320px 中英文 6 项浏览器检查通过；覆盖大额/小数/科学计数、全额代扣、配对分页、税前与实际到账、收税军团、方向筛选和军团不拆行，截图已查看。GitNexus 提交检查 15 文件、42 符号、low，实际调用范围为钱包读取和展示，未修改同步或账户数据写入。

应用与边缘 current 均为新版本，active/running/ready、NRestarts=0；首页/工作台/钱包 200，匿名上下文、汇总和收入/支出/交税筛选均 401。发布哈希与 OpenResty 配置检查通过，公网 HTML SHA256 `f13d973652a8becb0afd91cc0348501368ef416749188d77ba9ba8682d42a2c5`。数据库备份 `/var/backups/glorynavy/before-v0.1.0-wallet-tax-rows-20261017-20261016T182551Z.dump` 为 454292820 字节，数据库和匹配 env 备份均 0600，旧发布保留。

日志与截图在本地 `.local/wallet-tax-rows/`。测试容器已清理，本地预览已关闭；未创建生产测试流水或操作成员会话。回退前后端需保持同一收入筛选口径，原始存储与迁移不变。

## v0.1.0-wallet-tax-20261017 — 2026-10-17

前后端来自干净且已推送的 `main` 提交 `e928d5b0d77d129bcea821db2c465e8224d018a9`，生产分支守卫通过。个人钱包流水补显代扣税与收税军团，支出筛选包含正税额，新增交税筛选；金额保持原始到账值，余额、首页/贷款汇总不重复减税。无新迁移，Goose 90。

隔离 PostgreSQL 的 EVE 钱包、wallet 模块与钱包/军团范围宿主回归、server 编译及相关 go vet 通过。覆盖跨页税额、重复同步去重、精确大额与小数、日期/类型/收税军团筛选、撤权和原始汇总。前端类型、lint、构建、7 项语言/税额测试、1440px/375px/320px 中英文 6 项浏览器检查通过，桌面税额列表及窄屏详情截图已查看。GitNexus 对照上一版本检查 17 文件、13 符号、0 条索引调用链，low，无 partial/error；手动核对钱包查询和页面调用路径。

应用和边缘 current 均为新版本；active/running/ready、NRestarts=0，首页/工作台/钱包 200，匿名钱包上下文、汇总、交税记录接口均 401。发布清单、公网 HTML 和 OpenResty 配置检查通过，入口 SHA256 `b301ad6d96ce4e5cb5086badf1eb8ec6c4b6d0f1702504a3f5f8521f72a291d9`。数据库备份 `/var/backups/glorynavy/before-v0.1.0-wallet-tax-20261017-20261016T180115Z.dump` 为 454214672 字节，数据库与匹配 env 备份均 0600。旧发布保留，回退需同步前后端，否则新交税筛选会被旧后端拒绝。

日志和截图位于本地 `.local/wallet-missing/`，测试容器已清理，本地预览已停止。诊断只读生产税额与同步记录，未创建生产流水或使用成员会话；上线检查不冒充成员实际操作验收。

## v0.1.0-corporation-scope-20261017 — 2026-10-17

前后端来自干净且已推送的 `main` 提交 `a73343bb590cd249a72f7dbe830b90658ece67c2`，生产分支守卫通过。清理规则按已同步军团名称搜索、多选；名单同时限制军团业务范围，非站点管理员再与本人有效绑定角色所属军团取交集。清理开关不影响范围，原权限仍需满足；空名单没有业务军团选项，成员配置入口仍可访问。无新迁移，Goose 90。

隔离 PostgreSQL 中 access/identity 全套及宿主/server 回归已执行：旧考勤夹具增加配置名单后，考勤、军团边界、多角色和损失对象权限 4 项复验通过；其余宿主测试通过，相关 go vet 通过。前端类型/构建、7 项语言/access 测试、9 项中英文桌面/375px/320px 浏览器检查通过；lint 保留 4 条旧警告。检查名称搜索、选择去重、保存失败保留、非管理员入口、手机名称换行。EVE 全套的历史失败见 TEST-01，本轮未重跑该全套。GitNexus staged 检查 35 文件、73 符号、23 条调用链，权限公共边界标记 critical，已提示并核对范围；无 partial/error。

备份 `/var/backups/glorynavy/before-v0.1.0-corporation-scope-20261017-20261016T172121Z.dump` 为 454004679 字节，数据库和 env 备份权限 0600。上线后后端 active/running/ready、NRestarts=0；成员/军团运营/舰船损失页面 200，匿名清理 GET/PUT/POST、成员和福利/考勤/技能上下文 401。公网页面 SHA256 `0b1a66da148a1bd7912d034f594acdb57f4d6643a50f98efcc6d5202fa838f71` 与发布包一致，OpenResty 配置检查通过。生产清理策略仍关闭、名单 0、移除归属 0，未实际清理角色；管理员需先按名称配置名单。

日志和截图在本地 `.local/corporation-scope/`；测试容器已清理，本地预览已停止。回退旧版会撤销新增的业务范围限制，且继续遵守 Goose 90 角色归属保留的回退约束，见[清理指南](integrations/corporation-cleanup.zh-CN.md)。

## v0.1.0-corporation-cleanup-20261017 — 2026-10-17

前后端来自干净且已推送的 `main` 提交 `2e86a3d04dc3038e176f67719328867acc03457b`，生产分支守卫通过。Goose 90 增加按军团每周清理的策略、归属保留和审计；管理员在 `/members` 配置及预览，首次启用保存 7 天后执行，完成后每周一次。生产保持 disabled、空白名单、next_run_at 空、removed/skipped 0、归属保留记录 0；共享 River 的 `identity.corporation-cleanup.v1` 已完成 1 次空扫描。没有实际清理生产角色或创建测试业务记录。

应用和公网边缘 current 均已切换；active/running、ready、NRestarts=0、Goose 90。首页、`/members`、`/account` 均 200；新策略 GET/PUT、预览 POST、成员和贷款上下文匿名均 401。SHA256 清单、OpenResty `nginx -t` 和公网入口 HTML 一致性通过，HTML SHA256 为 `b4db1e628ef062fdcc8dafab96e4fb81913660a54afa9b915ff570563197064e`。备份 `/var/backups/glorynavy/before-v0.1.0-corporation-cleanup-20261017-20261016T164908Z.dump`（453883405 bytes）及同前缀环境备份均为 0600，旧发布保留。首次真实启用按 CLEANUP-01 验收。

独立 PostgreSQL 的 identity、宿主/server 回归通过；5 项清理专项覆盖按角色/主角色/最后角色、13 类不确定事实、账号及账本保留、失败回滚、25 账号批次续跑、归属恢复与合并、管理员/CSRF/版本检查、未知账号不修改及 Goose 90 Down 保护，相关 go vet 通过。前端类型/构建、7 项单元测试、9 项桌面/375px/320px 中英文浏览器检查、1 项原成员流程回归通过，截图已检查；lint 无错误，4 项既有警告保留。扩大 EVE 全量回归遇到 3 项基线失败及默认 10 分钟超时，失败均在未修改 `1081d98` 复现，记录 TEST-01，不声称全量通过。

GitNexus staged 检查识别 33 文件、13 条调用链，因应用启动/SSO/账号合并涉及范围标记 high，已提示并复核实际差异及上述专项边界。日志、截图和备份校验在本地 `.local/corporation-cleanup/`；测试容器已清理，本地预览已停止。回退前关闭规则并取消该 kind 的未结束作业；已产生保留归属后禁止回退不识别该表的程序或删除该表，详见[清理指南](integrations/corporation-cleanup.zh-CN.md)。

## v0.1.0-workspace-income-20261016 — 2026-10-16

前后端来自干净且已推送的 `main` 提交 `3a11d88bcb23ebe908c5e38345256b4a8afe8379`，生产分支守卫通过。工作台赚钱/花钱速度使用扣除直接转账后的精确月度汇总，先取每笔流水最新版本再按类型排除玩家捐助和三类军团直接转账。原始收支、余额、流水与贷款收支输入不变，无迁移，Goose 89；已有本月流水读取时重算。前后端一起发布，旧前端仍可读原字段；新前端遇到旧后端缺少统计字段时显示不可用。

应用与公网边缘 current 均已切换；应用 active/running、ready、NRestarts=0。首页、`/workspace`、`/wallet` 为 200，匿名钱包汇总、流水与贷款上下文均为 401。发布 SHA256 清单、OpenResty `nginx -t` 与公网工作台入口 HTML 一致性通过，入口 SHA256 为 `867f78455a0787b802713d992c492a24bbd97427537524b015eb4151fb642400`。数据库备份 `/var/backups/glorynavy/before-v0.1.0-workspace-income-20261016-20261016T153113Z.dump`（453590425 bytes）与同前缀环境备份均为 0600，旧发布保留。

独立 PostgreSQL 的 EVE 钱包、wallet 模块及宿主/server 测试、相关 go vet 通过；覆盖内部/外部转账、精确大额、流水变体、权限撤销和原始汇总兼容。前端类型/lint/构建、12 项单元测试、桌面/375px/320px 中英文 6 项浏览器检查与原完整钱包回归 1 项通过，截图已查看。GitNexus 提交检查为 low，实际差异已复核。日志与截图在本地 `.local/workspace-income/`，测试容器已清理、本地预览已关闭；未在生产创建测试数据或使用成员会话验算个人金额。

## v0.1.0-loan-auto-security-20261016 — 2026-10-16

前后端来自干净且已推送的 `main` 提交 `95864fda41dc455d3c975f6c109a38a1222ef8c3`，生产分支守卫通过。保障金额按超出可用个人额度的剩余缺口自动填写，办理中金额防止重复分配但不用于批准贷款；物品选择及交付均检查总估值的 80% 是否达到抵押目标。Goose 89 保存目标和选择估值，并允许拒绝/释放后重新邀请同一担保人。旧待交付记录保持原折扣兼容，产生新冻结目标或重复邀请历史后禁止回退忽略这些数据的程序/迁移。

应用与公网边缘 current 均已切换；应用 active/running、ready、NRestarts=0，Goose 89；首页、`/loans`、`/loans/operations` 均为 200，匿名上下文、详情、审批及新增 `POST /api/v1/loan/cases/1/collateral-quote` 均为 401。发布 SHA256 清单、OpenResty `nginx -t` 与公网入口 HTML 一致性通过，入口 SHA256 为 `5756e7bc63aa402ad864fe2dae5b34b69763ff1e9cb10a8bd6ea8cc413d14eea`。数据库备份 `/var/backups/glorynavy/before-v0.1.0-loan-auto-security-20261016-20261016T150523Z.dump`（453463177 bytes）与同前缀环境备份均为 0600，旧发布保留。

独立 PostgreSQL 贷款模块及宿主/server 测试、相关 go vet 通过；覆盖自动缺口、八折临界值/缺价、多物品合计、交付跌价、取消、同人重新邀请、旧记录阻塞、并发分配及审核。前端类型/lint/构建、7 项语言金额测试与桌面/375px/320px 中英文 18 项浏览器检查通过，截图已查看。日志与截图在本地 `.local/loan-auto-security/`。未在生产创建测试业务数据，独立测试容器已清理、本地预览已关闭；真实游戏资产及合同交付仍按 [LOAN-01](backlog.md) 联调。

## v0.1.0-loan-guarantor-details-20261016 — 2026-10-16

前后端来自干净且已推送的 `main` 提交 `757362cf3f6c0065543ee6ef3864c7c7391c893d`，生产分支守卫通过。详情接口在案件对象授权后批量解析已有担保角色，增加可选显示名称；页面逐笔显示担保人、精确金额与状态，名称缺失回退编号，历史缺角色时回退账号。担保归属、授信与决策审计不变，无新迁移，Goose 88。

应用与公网边缘 current 均已切换；应用 active/running、ready、NRestarts=0，首页与 `/loans`、`/loans/operations` 返回 200，匿名详情、上下文与审批返回 401。发布 SHA256 清单、OpenResty `nginx -t`、公网入口 HTML 与发布包一致性均通过，入口 SHA256 为 `07cce9dbfb6f014c28871cfef31158cff234f929624b76074dd9a830abae8971`。数据库备份为 `/var/backups/glorynavy/before-v0.1.0-loan-guarantor-details-20261016-20261016T135039Z.dump`（452012336 bytes），同前缀环境备份与数据库备份均为 0600，旧发布保留。未在生产创建测试业务数据，独立测试容器已清理，本地预览已关闭。

独立 PostgreSQL 贷款模块回归、宿主/server 编译与相关 go vet 通过，覆盖对象鉴权前不解析名称、批量去重、缺失/失败回退、JSON 字段兼容与读取不修改担保。前端类型、lint、构建和 5 项语言测试通过；1440px、375px、320px 中英文多笔担保身份/金额/状态与回退 6 项、原抵押流程 6 项通过，截图已核对。GitNexus 提交前检查为 medium（14 个文件、21 个索引符号、4 条详情读取/展示流程），实际差异已复核。日志与演示截图在本地 `.local/loan-guarantor-detail/`；真实游戏合同仍按[待办](backlog.md)验收。

## v0.1.0-loan-lender-details-20261016 — 2026-10-16（前端）

静态前端来自干净且已推送的 `main` 提交 `e78687fe90b02da724ad79cf5b967cda48faa7d9`，生产分支守卫通过。详情顶部显示出借方资金池名称、统一池标识和当前贷款专员，区分角色名暂缺、未配置和上下文不可用，长名称在窄屏完整换行。既有上下文与详情接口不变，无后端或迁移变更。

公网边缘 current 已切换，发布 SHA256 清单、OpenResty `nginx -t`、首页及 `/loans`、`/loans/operations` 的 200 响应通过；匿名上下文、案件详情与审批接口均为 401。公网入口 HTML 与发布包一致，SHA256 为 `96b04dcd2d1be72066886a9231b7e893edb18dc965ae9c9c6d669257d5cde518`。后端保持 `v0.1.0-loan-single-credit-20261016`、Goose 88，active/running、ready、NRestarts=0。仅切换静态发布，旧前端目录保留，未在生产创建测试业务数据。

前端类型检查、相关 lint、生产构建、5 项语言测试通过；1440px、375px、320px 中英文长名称/角色 ID 回退/未配置/缺少池上下文检查 6 项、原抵押流程 6 项通过。GitNexus 提交前检查为 medium（9 个文件、10 个索引符号、2 条详情翻译/格式化流程），实际差异已复核。截图和日志在本地 `.local/loan-lender-detail/`，采用演示数据，本地预览已关闭。实际游戏合同验收仍按[待办](backlog.md)执行。

## v0.1.0-loan-single-credit-20261016 — 2026-10-16

前后端来自干净且已推送的 `main` 提交 `2fab9287e2b7ffa4d8d55e3e560177a9e70bf2ad`，生产分支守卫通过。个人额度统一沿用 `total_limit_minor`，额度内免担保/抵押，仅超出剩余额度的本金需要本单有效担保或已批准抵押补足；新增信用占用和池内资金预留的事务锁及 `single-credit-v1` 审计，历史评估字段保留，Goose 88 无新迁移。回退旧审批代码会恢复旧双额度限制，不应以旧规则追溯重算新审批。

应用与公网边缘 current 均已切换；应用 active/running、ready、NRestarts=0，Goose 88；首页、`/loans`、`/loans/operations` 返回 200，匿名贷款上下文、评估记录 GET 和案件审批 POST 均为 401。发布 SHA256 清单、公网入口 HTML 与发布包一致性、OpenResty `nginx -t` 通过，公网入口 SHA256 为 `58e6d15c057f6be9c8d4d5312fda66ea3a1b12ad3fc9c96d0b1c5e5b9dfda825`。未在生产创建测试贷款、担保或抵押记录。

备份保留在 `/var/backups/glorynavy/before-v0.1.0-loan-single-credit-20261016-20261016T115125Z.dump`（451359502 bytes）及同名前缀 `.env`，均为 0600，旧发布目录保留。独立测试 PostgreSQL 容器已清理，本地预览已关闭。

独立 PostgreSQL 贷款模块、宿主应用和 server 检查及相关 go vet 通过；覆盖额度内免保障、超额的准确缺口、担保/抵押组合、未确认保障不计入、已有借款及对外担保占用、失效信用/资金池余额/权限、审计回滚，以及并发借款、担保和跨账号资金预留。前端类型检查、相关 lint、构建、7 项语言/金额测试通过；1440px、375px、320px 中英文单额度展示和超额申请提交 6 项通过，原资产抵押流程 6 项通过。窄屏检查发现并修复评估网格和读屏金额绝对定位造成的整页溢出，修复后复测通过。截图和日志位于本地 `.local/loan-single-credit/`。GitNexus 提交前检查为 medium（15 个文件、14 个索引符号、4 条受影响流程），实际 diff 已复核。模拟验证不代替真实游戏资产与合同验收，仍按[待办](backlog.md)联调。

## v0.1.0-loan-collateral-assets-20261016 — 2026-10-16

前后端来自干净且已推送的 `main` 提交 `08b3cc1ca298d7fde104b19b9be49748fbb2523f`，生产分支守卫通过。新增个人资产先选后交付、Goose 88 待交付记录与详情分组排版；合同匹配、占用、登记及审计原子提交，待交付不计算抵押覆盖。已有请求后禁止回退忽略请求的程序，迁移 Down 拒绝删除非空请求表。

应用与公网边缘 current 均已切换；应用 active/running、ready、NRestarts=0，Goose 88；首页、/loans、/loans/operations 返回 200，匿名贷款上下文、资产候选 GET、待交付创建 POST 与取消 POST 均为 401。发布 SHA256 清单、公网入口 HTML 与发布包一致性、OpenResty `nginx -t` 通过。公网入口 SHA256 为 `94d981718a874a92b5d83332863e2e7b248476b6c9aa9bb1f8205ee86d4cf930`。未在生产创建测试业务记录，发布核对时新请求表为 0 条。

备份保留在 `/var/backups/glorynavy/before-v0.1.0-loan-collateral-assets-20261016-20261016T102400Z.dump`（450914321 bytes）及同名前缀 `.env`，均为 0600，旧发布目录保留。独立测试 PostgreSQL 容器已清理，本地预览已关闭。

贷款模块、宿主应用和 server 数据库测试及相关 go vet 通过；覆盖归属、超选并发、同地点限制、错误合同/重复交付、审计回滚、其他绑定角色交付、申请撤销与合并。前端类型检查、相关 lint、构建、7 项语言/金额测试通过；中英文桌面/375px/320px 的还款、撤销和抵押流程通过，撤销冲突重试修复后 6 项重跑通过，最后翻译补齐后英文桌面/320px 2 项复测通过。截图位于本地 `.local/loan-collateral-detail/`，均为演示数据。GitNexus 提交前检查为 critical（26 个文件、169 个符号、45 条受影响流程），涉及贷款新增链路与宿主装配；已逐项核对实际 diff，并完成贷款及宿主回归。真实个人资产、装配/容器与游戏合同交付仍按待办联调，不把模拟测试或部署成功当作真实闭环验收。

## v0.1.0-loan-compact-layout-20261016 — 2026-10-16

仅公网静态前端发布，来源为干净且已推送的 `main` 提交 `894bb36dbde8388e9ec335c2a1ad376afa368682`。顶部额度/操作工具栏、全宽贷款列表、按实际宽度切换的担保/出借记录和 640px 表单已上线；看板及列表金额使用 M/B/T，完整金额保留在提示、读屏、详情和合同复制中。

边缘 current 已原子切换，136 个发布文件 SHA256、OpenResty `nginx -t`、首页/贷款办理页 200、匿名贷款上下文 401 通过；公网贷款入口 HTML 与发布包 SHA256 一致。应用仍为 `v0.1.0-loan-application-cancel-20261016`，active/running、ready、NRestarts=0；本次未重启应用或执行 Goose/River 迁移，Goose 87 不变，旧静态发布目录保留。

本轮前端类型检查、相关 lint、生产构建、5 项语言测试和 2 项金额格式测试通过；1440px/375px/320px 中英文 12 项申请、还款与撤销流程，以及含 1024px 平板的 8 项长编号/大金额/精确复制检查通过；最终 2 项桌面中英文流程另核对看板金额字号和截图。GitNexus 提交前影响检查为 low、0 条受影响执行流程，实际 diff 仅贷款前端与文档。截图与本地验收输出位于 `.local/loan-density/`，使用演示数据；本地预览已关闭，未在生产创建测试业务记录。

## v0.1.0-loan-application-cancel-20261016 — 2026-10-16

前后端来自干净且已推送的 `main` 提交 `86dcc46a70d695d93d3aede0882581313311f89f`。本轮新增本人待审核申请撤销，原子释放担保/抵押额度并保留审计，防止撤销后的保障资料迟到写入；贷款申请与出借表单精简，手机贷款记录直接显示操作。无新增迁移，Goose 保持 87。

激活前备份：`/var/backups/glorynavy/before-v0.1.0-loan-application-cancel-20261016-20261016T085608Z.{dump,env}`，数据库备份 450408385 字节，两者权限均为 0600。已核对应用和边缘 current、发布包 SHA256、二进制 vcs.revision/vcs.modified=false、active/running、NRestarts=0、ready、Goose 87、OpenResty 配置、公网首页和贷款办理页 200、匿名贷款读取与新增撤销接口 401。

本轮验证：隔离 PostgreSQL 18.6 的 `go test -p 2 ./internal/modules/loan/... ./internal/app ./cmd/server -count=1` 与相关 `go vet` 通过；覆盖撤销对象权限、终态与版本限制、释放额度、审计失败回滚、与审核并发、迟到抵押登记拒绝，以及既有还款流程。前端 tsc、相关 oxlint、生产构建、5 项语言测试通过；1440px/375px/320px 中英文共 12 项浏览器流程通过，补齐英文词条后 6 项英文流程再次通过。截图与构建输出位于本地 `.local/loan-cancel/`。GitNexus 单符号影响为 LOW（路由为 UNKNOWN）；提交前聚合检测报告 critical/22 条流程，部分未改函数受新增代码行号偏移误归属，已按实际 diff 复核，贷款付款逻辑未改。临时测试数据库容器和本地预览已关闭。

未在生产创建或撤销成员贷款进行测试，真实 EVE 合同及抵押物返还仍待业务联调；撤销只释放额度，既有游戏合同唯一占用及现金账本不变，不能把自动化演示数据当作真实交付验收。

## v0.1.0-loan-repayment-options-20261016 — 2026-10-16

前后端已从干净且已推送的 `main` 提交 `d599bdbdebc55348540f6d091a6deb038497a37e` 构建并发布；应用及边缘 `current` 均为该版本。新增 Goose 87 `loan_cases.requested_due_at`，支持按期限一次还清和指定日期一次还清，保留原分期。指定日期利息按合同实际完成日确定，贷款付款可精确结清 0.01 ISK；已有指定日期贷款后不得回退忽略日期的二进制，Down 会拒绝删除已有约定日期。

激活前备份为 `/var/backups/glorynavy/before-v0.1.0-loan-repayment-options-20261016-20261016T082409Z.{dump,env}`，数据库备份为 451237155 字节。已核对应用 active/ready、NRestarts=0、二进制 vcs.revision 与 vcs.modified=false、Goose 87、日期字段类型、边缘 SHA256 与 OpenResty 配置，公网首页和贷款办理页 200，匿名贷款读取/提交均为 401。

独立 PostgreSQL 18.6 测试覆盖申请、期限/日期持久化、审批日期投影、延迟放款、晚同步、历史合同拒绝及事务回滚、重复核验、精确本金/利息结清；贷款模块、宿主应用测试和相关 go vet 通过。前端类型检查、相关 lint、生产构建、5 项语言测试及桌面/375px/320px 中英文 6 项浏览器流程通过，并检查窄屏截图。未在生产创建测试业务记录，临时数据库容器已清理；真实游戏合同闭环仍待业务验收。附加后端词表生成检查发现既有 QQ/建筑等词条与生成文件不同，本轮没有新增后端文案，未纳入无关词表变更。

## v0.1.0-loan-submit-fix-20261016 — 2026-10-16

已发布后端，来源为已推送且干净的 `main` 提交 `7a2e72a52313934d5790be674b5c65c5b75b7e38`。修复借款申请 SQL 的金额参数推断和申请/付款角色 ID 的驱动编码问题；5 个 Go 二进制从该提交构建，前端复用上一发布包，公网边缘继续保持 `v0.1.0-loan-asset-evidence-20261014-r5`。

现有激活脚本在切换前完成数据库及配置备份，文件为 `/var/backups/glorynavy/before-v0.1.0-loan-submit-fix-20261016-20261016T073829Z.{dump,env}`。应用已切换至新目录；`active/ready`、`NRestarts=0`、二进制 `vcs.revision` 与 `vcs.modified=false`、Goose 86 均已核对，无新增迁移。公网首页和 `/loans/operations` 为 200，匿名 `GET /api/v1/loan/context` 与 `POST /api/v1/loan/applications` 为 401。

独立 PostgreSQL 18.6 测试库复现旧申请 503，修复后申请提交、冻结条款、审计、未绑定角色拒绝、待核验出借及放款/还款记录创建通过；贷款模块、宿主应用测试与相关 `go vet` 通过。没有在生产库创建测试借款、出借或付款记录，真实成员提交与合同交付仍需业务验收。

## v0.1.0-loan-asset-evidence-20261014-r5 — 2026-10-14

修复资产授信证据被 ESI 大响应体限制清零的问题：资产分页和全服市场均价表使用受控的 8 MiB 响应上限，未报价物品跳过并保留其他可报价资产，部分角色读取失败记录在快照证据中。规则版本递增后 River 自动重评估现有账号；最新 29 条快照均为 `capacity-v6`，其中 28 条有正资产估值、24 条产生资产支持额度，1 条仍需 EVE 授权续期。Goose 保持 86，应用服务 active、`/health/ready` ready，贷款队列评估任务完成；公网边缘 current 已切换同名版本，首页和贷款页面 200，OpenResty `nginx -t` 通过。贷款模块、宿主应用和 EVE 模块测试通过。

## v0.1.0-loan-periodic-credit-20261014 — 2026-10-14

应用机已切换修正版后端并执行 Goose 86；生产策略默认资产 LTV 为 1000 bps、钱包样本门槛为 1、证据不足策略为 zero，周期性信用评估表已生成 29 条快照，服务 active、/health/ready 返回 ready。应用修正版发布包来源为本轮贷款额度、周期任务和抵押自动估值改动。公网边缘已通过应用机跳板完成静态前端原子切换，当前目录为 v0.1.0-loan-periodic-credit-20261014；OpenResty 配置检查通过，首页和贷款页面返回 200。


## v0.1.0-loan-credit-income-consumption-20261014 — 2026-10-14

后端应用和公网静态前端已切换至同名版本；后端来源提交 `714ddc8`，前端来源提交 `3f7a499`。Goose 85 新增收入能力权重（生产值 7000 bps），个人授信分别汇总收入和消费，不再计算净收入，也不再读取或写入个人硬上限。贷款模块与宿主应用测试通过，应用 `active`、`/health/ready` 200，数据库策略读取为 `income_weight_bps=7000`，公网首页和 `/loans/operations?view=risk` 200，匿名 `/api/v1/loan/context` 401，OpenResty `nginx -t` 通过。

## v0.1.0-loan-credit-zero-income-20261014 — 2026-10-14

后端应用已切换至 `v0.1.0-loan-credit-zero-income-20261014`，代码来源提交 `4497b3c`；公网静态前端保持 `v0.1.0-loan-credit-cleanup-20261014`。零或负月净收入按收入不足策略处理，默认回退到贷款池资金、个人基础上限和信用评分。贷款模块测试通过，发布包校验通过，应用 `active`、`/health/ready` 200，公网首页与贷款办理页 200，匿名贷款上下文接口 401；本次无新增迁移。

发布脚本首次执行的远程命令会话提前结束，服务停下后已恢复旧版本并确认就绪；第二次完整等待备份与原子切换后发布成功。标准数据库与配置备份由激活脚本保存在 `/var/backups/glorynavy/`，之前的应用发布目录保留。只读核对确认管理员入池资金已记账，登记账号的收入样本满足门槛且新鲜，但月净收入为 0，符合此次修复分支。当前登录页面的最终额度数字未完成浏览器读取验收。

## v0.1.0-loan-credit-cleanup-20261014 — 2026-10-14

已发布来源提交 `404de96`，公网静态站点已原子切换至 `v0.1.0-loan-credit-cleanup-20261014`。贷款办理页移除额度为 0 时的通用原因说明和“暂不可用”状态标记，仅保留信用评分与个人额度。后端、数据库、Goose、River 和应用服务未修改；首页、`/loans/operations?view=business`、`/loans/operations?view=dashboard` 返回 200，匿名贷款上下文接口返回 401，OpenResty `nginx -t` 通过。

## v0.1.0-loan-dashboard-20261013-r7 — 2026-10-13

已发布来源提交 `9125034`，当前边缘目录为 `v0.1.0-loan-dashboard-20261013-r7`。贷款办理页移除重复的“查看担保邀请”按钮，担保人直接在下方列表查看并处理邀请；申请与出借入口调整为双列操作卡片。本次只切换公网静态前端，没有重启应用服务、没有执行 Goose/River 迁移。首页和 `/loans/operations` 返回 200，匿名贷款接口保持 401，容器内 OpenResty `nginx -t` 通过。

## v0.1.0-loan-dashboard-20261013-r6 — 2026-10-13

已发布来源提交 `0c3f3bf`，当前边缘目录为 `v0.1.0-loan-dashboard-20261013-r6`。申请、担保和出借入口在办理页合并为一个外层操作卡片，卡内并列动作。本次只切换公网静态前端，没有重启应用服务、没有执行 Goose/River 迁移。首页、`/loans`、`/loans/operations` 返回 200，匿名 `/api/v1/loan/context` 返回 401，容器内 OpenResty `nginx -t` 通过。

## v0.1.0-loan-dashboard-20261013-r5 — 2026-10-13

已发布来源提交 `024079f`，当前边缘目录为 `v0.1.0-loan-dashboard-20261013-r5`。信用评分、额度和评分因子仅在“贷款办理”子页面顶部展示，“贷款看板”只展示整体借贷信息。本次只切换公网静态前端，没有重启应用服务、没有执行 Goose/River 迁移。首页、`/loans`、`/loans/operations` 返回 200，匿名 `/api/v1/loan/context` 返回 401，容器内 OpenResty `nginx -t` 通过。

## v0.1.0-loan-dashboard-20261013-r4 — 2026-10-13

已发布来源提交 `2da68e5`，当前边缘目录为 `v0.1.0-loan-dashboard-20261013-r4`。“贷款业务”改名为“贷款看板”，统一贷款池整体指标集中在看板；个人信用、额度和评分因子移到贷款办理页顶部，申请、担保邀请和出借入口合并为单个操作卡片。本次只构建并切换公网静态前端，没有重启应用服务、没有执行 Goose/River 迁移。首页、`/loans`、`/loans/operations` 返回 200，匿名 `/api/v1/loan/context` 返回 401，容器内 OpenResty `nginx -t` 通过；本机 Playwright 验收脚本因缺少浏览器二进制未能启动。

## v0.1.0-loan-dashboard-20261013-r3 — 2026-10-13

已发布来源提交 `70c77f8`。侧栏在“财务与工具”下保留“贷款”入口，`/loans/operations` 内新增默认的“贷款业务”子页面，贷款申请、风控设置和评估记录继续作为同页子页面；`/loans` 兼容跳转。后端上下文接口仅向管理员或统一池管理者读取真实资金池汇总，普通用户收到零值汇总且只展示本人信用与额度。应用机在生产 Go 环境构建 Linux/amd64 二进制，贷款模块测试通过；本次无 Goose/River 新迁移。应用机与公网静态站点均已原子切换，应用 `active/ready`、`NRestarts=0`、匿名贷款接口 401，首页、`/loans`、`/loans/operations` 返回 200，OpenResty `nginx -t` 和浏览器部署验收通过。

## v0.1.0-loan-dashboard-20261013-r2 — 2026-10-13

已发布来源提交 `b19d32f`。主导航新增独立“贷款”分组，贷款业务看板与贷款办理为其两个子页面；路由保持 `/loans` 和 `/loans/operations`。本次只切换公网静态前端，没有重启应用服务，也没有执行 Goose/River 迁移。当前边缘目录为 `v0.1.0-loan-dashboard-20261013-r2`，首页、两个贷款页面返回 200，容器内 OpenResty `nginx -t` 通过，浏览器部署验收脚本通过。

## v0.1.0-loan-dashboard-20261013-r1 — 2026-10-13

已发布来源提交 `35691d8`。本次只构建并切换公网静态前端：`/loans` 改为紧凑贷款业务数据看板，`/loans/operations` 保留贷款办理并增加返回看板入口，操作卡片移除重复长说明。没有重启应用服务，也没有执行 Goose/River 迁移。边缘发布脚本校验 `main` 干净来源、SHA256SUMS 并完成原子切换；当前版本目录为 `v0.1.0-loan-dashboard-20261013-r1`，公网首页、`/loans`、`/loans/operations` 均返回 200，容器内 OpenResty `nginx -t` 通过，`node scripts/check-deployment.mjs https://seat.kisectool.com` 浏览器验收通过。

## v0.1.0-loan-dashboard-20261013 — 2026-10-13

已发布来源提交 `214693e`。贷款年利率、1/3/7/14/30 天还款间隔、放款后首期到期时间、担保责任额度复核、抵押合同选择和贷款业务/贷款办理双页面已上线；应用机与公网静态站点均已原子切换。激活脚本完成数据库与配置备份，生产 Goose 80–82 已执行，应用 `active/ready`、`NRestarts=0`；公网首页和 `/loans` 返回 200，匿名贷款及系统接口返回 401，OpenResty `nginx -t` 通过。`node scripts/check-deployment.mjs https://seat.kisectool.com` 使用本机 Chrome 完成真实登录表单、PKCE/scopes、Secure Cookie、Origin/CSRF 和匿名隔离检查。

## v0.1.0-loan-policy-help-20261012 — 2026-10-12

已发布来源提交 `8aad5a0`。风控设置字段旁新增可访问的小问号说明，涵盖 bps 换算、收入样本、抵押估值基数和数据不足处理；支持桌面悬停/键盘聚焦及移动端点击关闭。本次仅更新公网静态前端，未重启应用服务、未执行 Goose/River 迁移。边缘发布脚本完成原子切换，容器内 OpenResty `nginx -t` 通过；正式域名首页和 `/loans` 返回 200，贷款上下文/案件接口匿名返回 401。

## v0.1.0-loan-credit-policy-20261012 — 2026-10-12

已发布来源提交 `f4a7c65`。贷款页新增风控设置与评估记录标签页，个人授信可按有效个人钱包净收入及可配置规则计算，担保/抵押覆盖上限和贷款专员可配置。应用机激活已执行 Goose 79/River，`glorynavy` 与数据库服务 active，`/health/ready` 200；生产 Goose 79、统一贷款策略单例存在。公网静态站点已原子切换，首页和 `/loans` 200、匿名贷款接口 401，OpenResty `nginx -t` 通过。

## v0.1.0-loan-pap-split-20261012 — 2026-10-12

已发布来源提交 `b9f2d9c`。贷款信用评分拆分 PAP 双因子：近一年军团活动集结率 3 分、近六个月联盟 PAP 满足情况 2 分；新增评估接口字段和页面因子展示，资产规模评分固定 0/10。无 Goose/River 新迁移；应用 `/health/ready` 200、匿名贷款接口 401、首页 200，边缘 OpenResty 配置检查通过。

## v0.1.0-loan-score-labels-20261012 — 2026-10-12

已发布来源提交 `2d7475c`。贷款页将“PAP 辅助”“资产规模辅助”统一改为“PAP 评分”“资产规模评分”，并展示 PAP 评分计算说明；本次仅更新公网静态前端，边缘 OpenResty 配置检查、发布脚本和首页 200 均已通过，应用服务保持上一版本运行。

## v0.1.0-operations-tax-range-transition-20261012 — 2026-10-12

已发布来源提交 `070a96e`。自定义税收排行日期输入改为标题区域内平滑展开，隐藏时不占布局空间且不可聚焦，移动端自动换行。本次仅更新公网静态前端；边缘 OpenResty 配置检查、发布脚本和首页 200 均已通过，应用服务保持上一版本运行。

## v0.1.0-operations-tax-range-20261012 — 2026-10-12

已发布来源提交 `35b6e9b`。运营面板税收排行支持最近一天、近三天、近七天、近一个月和自定义日期范围；应用与公网静态站点已原子切换，应用 `active/ready`、排行接口匿名 401、首页 200、边缘 OpenResty 配置检查和发布脚本均已通过。

## v0.1.0-operations-tax-ranking-20261012 — 2026-10-12

已发布来源提交 `e8873ab`。运营面板新增近六个月个人税收排行，按本站账号合并当前军团有效角色，显示前十昵称与交税金额；新增只读钱包排行接口，无 Goose/River 新迁移。应用机与公网静态站点已原子切换，应用 `active/ready`、排行接口匿名 401、首页 200 和边缘发布脚本验收通过。

## v0.1.0-approval-applicant-nickname-20261012 — 2026-10-12

已发布来源提交 `844e1c8`。审批中心申请人列表和筛选改用账号昵称投影，未设置昵称时回退来源名称；无 Goose/River 新迁移。应用机与公网静态站点已原子切换，应用 `active/ready`、匿名审批接口 401、首页 200 和边缘 `nginx -t` 均已复核。

## v0.1.0-approval-detail-contract-20261011-r4 — 2026-10-11

应用机将切换至 r4，修复审批详情 `actions: null` 导致前端响应格式校验失败的问题；详情统一保证 `actions` 为数组、`payload` 为空时为对象。公网静态站点继续使用 r3，无 Goose/River 新迁移。审批模块测试通过。

## v0.1.0-approval-history-fast-20261011-r3 — 2026-10-11

公网静态站点已切换至 r3。已处理视图在标签悬停/聚焦时预取，并在当前列表可用后后台预取历史筛选结果；历史查询缓存窗口为 30 秒。仅前端静态资源变更，应用服务保持 r1，无 Goose/River 新迁移。前端检查（Go 全量测试、96 项前端测试、lint、构建）通过，lint 仅保留既有警告。

## v0.1.0-approval-detail-fast-20261010-r1/r2 — 2026-10-10

应用机已切换至 r1，公网静态站点已切换至 r2。审批详情由来源单条读取或 `approval_items` 单行读取，详情接口生产样本 267–306ms；前端详情组件支持悬停/聚焦预加载并以列表摘要占位。Go 全量测试、前端 96 项测试、lint 和构建通过；应用 active/ready、匿名审批详情 401、首页 200、容器内 OpenResty `nginx -t` 通过。无 Goose/River 新迁移。

## 审批投影契约对齐与双读验收（2026-10-10）

已发布 `v0.1.0-approval-read-model-20261010-r23`，来源提交 `a613257654d22748569420c37112a30358ec6424`。Goose 78 新增 `approval_items.action`，福利成长/活动奖励单位、三类来源最新处理动作和审计时间与旧来源查询保持一致，修复历史分页顺序和奖励单位双读差异。应用机 `/opt/glorynavy/current` 与公网 1Panel 静态站点已原子切换；应用 `active/ready`、首页 200、Goose 78、三来源投影 `exchange=fresh/0`、`loan=fresh/0`、`welfare=fresh/109` 和边缘 `nginx -t` 均已复核。

生产真实管理员会话覆盖待审批、待发放、异常、已处理四个视图，`APPROVAL_DUAL_READ=true` 验收窗口没有新增 `approval_dual_read_diffs`，随后已恢复关闭。验收窗口索引整页样本为：待审批 p95 248ms、待发放 p95 259ms、异常 p95 203ms、已处理 p95 21ms；福利旧聚合查询约 2.9–3.1s。Go 全量测试通过，前端 28 个测试文件/96 个测试和构建通过。

## 审批来源扩展契约安全边界（2026-10-10）

已发布 `v0.1.0-approval-read-model-20261010-r19`，来源提交 `2f8d32a`。统一列表前端响应校验接受任意已注册来源，未知来源摘要按通用字段展示；缺失福利详情不会让列表渲染失败。`IndexOnly` 来源详情没有旧查询适配器时安全返回来源不可用。全量 Go 测试、前端测试与构建通过；应用机和边缘静态站点已切换，服务 ready、首页 200、匿名审批接口 401、三来源投影 fresh、OpenResty `nginx -t` 均已复核。

## 审批首屏轻量上下文授权（2026-10-10）

已发布 `v0.1.0-approval-read-model-20261010-r18`，来源提交 `b176b50`。福利来源的审批上下文 shell 使用轻量 `ContextAccess`，不在初次读取加载完整成员账号范围；申请人筛选和索引列表继续使用完整来源授权。应用与公网静态站点已切换，应用 `active/ready`、首页/静态资源 200、匿名审批接口 401、三来源投影 fresh 和 OpenResty `nginx -t` 均已复核。

## 审批索引路径与首页启动兜底（2026-10-10）

已发布 `v0.1.0-approval-read-model-20261010-r16`，来源提交 `664b58e`。审批索引列表增加回归测试，确认正常索引路径不会调用来源旧分页查询；首页增加主脚本或懒加载资源失败时的静态启动/重载提示，并兼容缺少 `ResizeObserver` 的浏览器。应用机与公网静态站点已切换，应用 `active/ready`、首页和静态脚本 200、匿名审批接口 401、Goose 77、三来源投影 fresh、OpenResty `nginx -t` 和真实浏览器首页无控制台错误均已复核。

## 审批双读并发保护（2026-10-09）

已发布 `v0.1.0-approval-read-model-20261009-r15`，来源提交 `3ea9174`。双读开启时后台旧聚合比较最多同时运行 4 个，超出请求跳过并记录原因；整体失败写入 `legacy_error` 诊断。无 Goose/River 新迁移；应用机与公网静态站点已切换，应用 `active/ready`、首页 200、匿名审批接口 401、Goose 77、投影状态和 OpenResty `nginx -t` 均已复核。

## 审批双读链路契约测试（2026-10-09）

已发布 `v0.1.0-approval-read-model-20261009-r14`，来源提交 `30d4602`。审批索引通过内部存储接口注入，双读异步比较和差异持久化拥有独立契约测试；生产仍使用 PostgreSQL 实现。无 Goose/River 新迁移；应用机与公网静态站点已切换，应用 `active/ready`、首页 200、匿名审批接口 401、Goose 77、投影状态和 OpenResty `nginx -t` 均已复核。

## 审批来源耗时诊断（2026-10-09）

已发布 `v0.1.0-approval-read-model-20261009-r13`，来源提交 `96b3aa5`。审批上下文和列表保留整页耗时，并新增来源级授权、成员筛选选项、旧查询、索引查询和名称投影阶段日志；日志不含账号、筛选词或业务 payload。无 Goose/River 新迁移；应用机与公网静态站点已切换，应用 `active/ready`、首页 200、匿名审批接口 401、Goose 77、投影状态和 OpenResty `nginx -t` 均已复核。

## 审批列表共享授权范围（2026-10-09）

已发布 `v0.1.0-approval-read-model-20261009-r12`，来源提交 `3844b74`。无独立 `IndexAccess` 的来源会在审批上下文和索引列表之间复用 5 秒授权范围；显式独立索引范围仍使用隔离缓存键。无 Goose/River 新迁移；应用机与公网静态站点已切换，应用 `active/ready`、首页 200、匿名审批接口 401、Goose 77、三来源投影状态和 OpenResty `nginx -t` 均已复核。

## 审批列表授权范围缓存（2026-10-09）

已发布 `v0.1.0-approval-read-model-20261009-r11`，来源提交 `19d4320`。应用机 `/opt/glorynavy/current` 与公网 1Panel 静态站点 `current` 已原子切换；无 Goose/River 新迁移。审批上下文和索引列表按账号、来源及读取路径隔离复用 5 秒授权范围结果，详情、审批决定和写操作仍实时回源。应用 `active/ready`、首页 200、匿名审批接口 401、Goose 77、`approval_projection_runs` 三来源状态和边缘 OpenResty `nginx -t` 均已复核。

## 审批中心统一读模型与首页白屏修复（2026-10-09）

已发布 `v0.1.0-approval-read-model-20261009-r10`，来源提交 `71f7cfb`。应用机 `/opt/glorynavy/current` 与公网 1Panel 静态站点 `current` 已原子切换；Goose 77 已执行，River 无新增迁移。审批中心统一列表使用 `approval_items` 索引，福利、兑换、贷款投影均为 `fresh`；投影记录为 `exchange=0`、`loan=0`、`welfare=109`，索引共 109 条。列表响应新增来源状态、摘要版本、详情能力和陈旧标记；详情通过来源注册表加载，未知来源回退通用摘要，`IndexOnly` 来源可安全接入索引。详情和审批写入仍回源来源模块执行权限、版本、合同与账务校验，双读诊断开关保持关闭，待管理员会话灰度验收。双读差异记录现覆盖来源集合、计数、游标、分页行权限字段、状态、处理人、金额和动作，并持久化到 `approval_dual_read_diffs`；`APPROVAL_INDEX_ACCOUNTS` 可按账号启用索引灰度。统一索引列表不再逐行调用兑换 SDE 名称服务，来源能力声明在启动时校验。

发布期间定位并修复 `App.tsx` 根路径自重定向导致的首页白屏，公开首页现直接渲染介绍和 EVE SSO 入口；真实浏览器复核首页内容、控制台错误和 `/login` 回到 `/` 均通过。应用 `active/ready`、公网首页和静态资源 200、匿名审批接口 401、OpenResty `nginx -t` 均已复核；上一版本保留在两台机器的 `releases/` 目录。

兼容登录地址的受保护页面跳转会保留查询参数，SSO 取消或失败原因可在首页入口显示；前端构建、单元测试和 lint 已复核。端到端测试需要本地 PostgreSQL，当前开发机未启动 `127.0.0.1:55432`，未将其误记为通过。

## 审批中心与贷款页面性能优化（2026-10-09）

已发布 `v0.1.0-approval-perf-20261009`。来源代码提交 `954ea34`；应用与公网静态站点已切换。审批上下文默认跳过申请人名称投影，展开“更多筛选”时按需读取；页签预读仅保留键盘聚焦，结算和上下文刷新按当前视图降频；贷款页面缓存键加入账号隔离并关闭窗口聚焦重复刷新。无 Goose/River 新迁移；应用 `active/ready`、首页 200、审批接口匿名 401、OpenResty `nginx -t` 通过。

## 移除独立登录页发布（2026-10-09）

已发布 `v0.1.0-home-login-20261009`，来源提交 `ecaf7f9`。公开首页 `/` 现在是唯一 EVE SSO 登录入口，旧 `/login` 地址由前端兼容重定向到首页，认证接口、SSO 回调和 `/account` 未变。此次仅更新公网静态前端，应用与数据库未变；边缘 `current` 已原子切换，首页 200、OpenResty `nginx -t` 通过。

## 审批中心分页加载状态修复（2026-10-09）

已发布 `v0.1.0-approval-pagination-loading-20261009`，来源提交 `4a7740d`。审批中心切换分页、筛选或排序后，新请求完成前清空旧页行并显示读取状态；已预取页面继续复用缓存。仅更新公网静态前端，应用与数据库未变；边缘 `current` 已原子切换，首页 200、OpenResty 配置检查通过。

## 贷款审批来源类型比较修复（2026-10-09）

已发布 `v0.1.0-approval-loan-queue-fix-20261009`，来源提交 `ad56ca1`。生产日志定位贷款审批聚合 SQL 的 `text = uuid` 类型错误，并修复前端将未知来源误标为奖励兑换的问题；贷款来源现在可正常查询并显示正确标签。无新增 Goose/River 迁移；应用机与公网静态站点已原子切换，应用 `active/ready`、首页 200、相关 Go 测试包和 OpenResty 配置检查通过。

## 审批中心待办名称解析降级发布（2026-10-09）

已发布 `v0.1.0-approval-task-name-fallback-20261009`，来源提交 `0e9e330`。审批中心申请人昵称和福利成员主角色名称解析改为展示增强；名称投影失败时仍返回待办、计数和接收角色，并记录来源访问/查询失败阶段。无新增 Goose/River 迁移；应用机与公网静态站点已原子切换，应用 `active/ready`、首页 200、相关 Go 测试包和 OpenResty 配置检查通过。

## 审批中心奖励兑换名称解析降级发布（2026-10-08）

已发布 `v0.1.0-approval-exchange-name-fallback-20261008`，来源提交 `0bbc2e41d5328b91a05ab2a55c9630280656229b`。兑换审批列表在 SDE 名称服务暂时不可用时继续使用冻结快照返回类型 ID、数量、金额和计数，不再把展示名称解析失败当作整个兑换来源不可用。无新增 Goose/River 迁移；应用机与公网静态站点已原子切换，应用 `active/ready`、首页/登录 200、匿名审批接口 401、容器静态入口和 OpenResty `nginx -t` 均通过。

## 补损专员独立权限发布（2026-10-08）

已发布 `v0.1.0-welfare-compensation-officer-20261008`，来源提交 `2120a942983bc34c1230699a9a8b712279677d20`。新增 `corporation.welfare.compensation` 独立站点权限，审批中心仅向该角色开放 `srp`/`solo` 补损审批与交付核验；普通成员权限保留，完整 `corporation.welfare` 范围不变。无新增 Goose/River 迁移；应用机与公网静态站点已原子切换，应用 `active/ready`、首页/登录 200、匿名福利接口 401、容器静态入口和 OpenResty `nginx -t` 均通过。

## 贷款详情移除核验按钮发布（2026-10-08）

已发布 `v0.1.0-loan-no-verify-button-20261008`。贷款出借详情移除自动核验按钮，合同完成并同步后由系统自动核验；详情只保留合同复制信息和取消出借操作。无新增 Goose/River 迁移。

## 贷款仅自动合同核验发布（2026-10-08）

已发布 `v0.1.0-loan-auto-only-20261008`。贷款出借的手动合同核验入口、接口和 OpenAPI 契约已移除，详情只保留自动核验和必要合同复制信息。无新增 Goose/River 迁移；应用与公网静态站点均已切换，应用 `active/ready`、首页与 `/loans` 200、匿名贷款接口 401、OpenResty `nginx -t` 通过。

## 出借资金列表与详情弹窗发布（2026-10-08）

已发布 `v0.1.0-loan-list-detail-20261008`。出借资金改为列表行，点击后打开详情弹窗；合同复制只保留托管对象 ID、支付金额和合同备注。无新增 Goose/River 迁移；应用与公网静态站点均已切换，应用 `active/ready`、首页与 `/loans` 200、匿名贷款接口 401、OpenResty `nginx -t` 通过。

## 贷款合同复制字段布局发布（2026-10-08）

已发布 `v0.1.0-loan-contract-copy-layout-20261008`，后续由仅自动核验版本覆盖。贷款页出借待核验区域的复制字段布局已保留为详情弹窗基础样式。无新增 Goose/River 迁移。

## 出借合同信息复制与自动核验发布（2026-10-08）

已发布 `v0.1.0-loan-contract-auto-verify-20261008`。贷款页会展示可复制的纯 ISK 物品交换合同信息，并按当前账号最近同步的完成合同自动匹配核验。手动合同核验已由后续版本移除。

## 贷款角色昵称下拉选择发布（2026-10-08）

已发布公网静态前端 `v0.1.0-loan-character-picker-20261008`。出借和收款角色改为从当前账号已绑定角色昵称下拉框选择，提交时仍由系统传递角色 ID；出借记录显示昵称。后端、数据库和应用服务未变；公网首页/贷款页 200，OpenResty `nginx -t` 通过。

## 贷款本金池按实际现金动态计算发布（2026-10-08）

已发布 `v0.1.0-loan-pool-cash-limit-20261008`。贷款总责任额度改为读取统一池账本的已核验现金余额并按信用分动态计算，固定 `max_principal_minor` 不再参与额度计算；待核验出借承诺和网站余额不计入本金池。应用机与公网静态站点均已原子切换，生产统一池当前现金余额为 `0`。应用 `active/ready`、Goose 73、首页与 `/loans` 200、匿名贷款接口 401、OpenResty `nginx -t` 通过。GitHub 推送因外网连接失败未完成，本次发布使用本地已提交的 `main` 构建包，后续网络恢复后需补推 `main`。

## PLEX 月卡自动核价修复发布（2026-10-08）

已发布 `v0.1.0-plex-average-20261008`。应用机与公网静态站点均已原子切换；PLEX 改查 EVE Global PLEX Market，订单簿缺边时使用 ESI 官方平均价作为中间价，普通物品的双边报价校验保持不变。无 Goose/River 新迁移；应用 `active/ready`、月卡重算、首页/登录和匿名鉴权检查通过，OpenResty `nginx -t` 通过。

## 同步资源类型响应校验修复发布（2026-10-08）

已仅更新公网静态前端至 `v0.1.0-sync-shape-fix-20261008`。角色同步接口返回 `corporation_structures` 目标时，前端现在能够通过响应校验并显示“军团建筑”；后端、数据库、Goose/River 和应用服务未改。公网 `current` 已原子切换，首页/登录 200，OpenResty `nginx -t` 通过。

## 联盟 PAP 管理员成员积分列表发布（2026-10-08）

已发布 `v0.1.0-alliance-pap-members-20261008`。应用机 `/opt/glorynavy/current` 与公网静态站点 `current` 已原子切换；无 Goose/River 新迁移，生产 Goose 72。应用 `active/ready`、首页 200、新成员接口匿名 401、模块目录匿名 401、OpenResty `nginx -t` 通过。应用机切换前数据库与配置备份为 `/var/backups/glorynavy/before-v0.1.0-alliance-pap-members-20261008-20261007T182116Z.{dump,env}`。真实管理员成员列表和多角色绑定边界仍按 PAP-03 现场验收。

## 联盟 PAP 本地快照成员查看修复（2026-10-08）

已发布 `v0.1.0-alliance-pap-local-snapshot-20261008`。管理员成员区新增完整快照月份选择，并在上游最近失败时继续读取本地快照；应用机与公网静态站点已原子切换。无 Goose/River 新迁移，生产 Goose 72；应用 `active/ready`、首页 200、匿名成员月份/成员接口 401、静态资源包含新接口及边缘配置检查均通过。生产当前保留 2026-10 22 条和 2026-09 74 条本地快照；真实管理员登录后的页面验收仍按 PAP-03 进行。

## 联盟 PAP 月份选择器样式统一（2026-10-08）

已发布 `v0.1.0-alliance-pap-selector-20261008`。公网静态站点已切换，联盟 PAP 成员月份选择器改用全站统一 Select 控件；无 Goose/River 新迁移，首页 200、资源加载和边缘配置检查通过。

## 审批中心读取优化发布（2026-10-08）

已发布 `v0.1.0-approval-auth-cache-20261008`。应用机 `/opt/glorynavy/current` 与公网静态站点 `current` 已原子切换；无 Goose/River 新迁移。审批上下文、列表和详情复用本次请求已取得的来源授权结果，并在创建批量结算后失效结算批次缓存。应用 `active/ready`、重启次数 0、公网首页/登录 200、匿名模块目录和系统状态 401，公网 OpenResty `nginx -t` 通过。生产加载耗时改善仍需真实管理员会话复测。

## 贷款申请与出借操作栏发布（2026-10-07）

已发布 `v0.1.0-loan-action-bar-20261007`，应用机与公网静态站点均已原子切换，来源提交为 `0b298e4`。申请和出借按钮共用一个无卡片操作栏，原有表单弹窗和权限逻辑保持不变。应用 `active/ready`、公网首页与 `/loans` 200、匿名模块和贷款接口 401，边缘 OpenResty `nginx -t` 通过。

## 贷款担保空列表响应修复（2026-10-07）

已发布 `v0.1.0-loan-guarantee-array-20261007-r2`，应用机与公网静态站点均已原子切换，来源提交为 `51fabea`。修复无待确认担保时 `GET /api/v1/loan/guarantees` 将空切片序列化为 `null` 的问题，接口现在返回 `items: []`，前端不再误报响应格式异常。Goose 72 保持不变；应用 `active/ready`、公网首页与 `/loans` 200、匿名模块和贷款接口 401，边缘 OpenResty `nginx -t` 通过。

## 系统信用评估与担保抵押发布（2026-10-07）

已发布 `v0.1.0-loan-credit-security-20261007`，应用机与公网静态站点均已原子切换，来源提交为 `5851acfb5df49d9734b389b9e7f73d44f047f8ce`。Goose 72 已执行；应用 `active/ready`、公网首页与 `/loans` 返回 200、匿名模块目录返回 401，边缘 OpenResty `nginx -t` 通过。发布前数据库与配置备份由激活脚本保存到应用机 `/var/backups/glorynavy/`。本地部署脚本的 Playwright 浏览器检查因缺少 Chromium 可执行文件未完成，远程就绪、静态入口和匿名鉴权检查已通过；真实 Tranquility 合同、成员/管理员操作和履约数据仍需按 [贷款待办](backlog.md) 现场验收。

## 固定统一贷款池初始化修复（2026-10-08）

已发布 `v0.1.0-loan-pool-seed-20261008-r1`，Goose 73 已执行。迁移从当前军团角色快照中选择角色数量最多的托管军团，建立唯一开放统一池；不再要求管理员创建或配置池。修复前生产缺少 `loan_pools` 记录，导致贷款页两个提交按钮被前端按空池禁用。应用 `active/ready`、Goose 73、唯一开放池、首页与 `/loans` 200、匿名贷款接口 401、OpenResty `nginx -t` 已复核。

## 统一贷款池固定化（2026-10-06）

该阶段代码随后随 `v0.1.0-loan-credit-security-20261007` 发布。贷款页移除统一贷款池创建/配置入口和人工授信入口，借款申请自动使用全站唯一贷款池；成员提交个人/军团出借金额后，仍须完成托管合同核验才进入池现金。信用评分、总责任额度和无担保额度由系统按履约记录评估，审批时重新计算。真实 Tranquility 合同闭环仍待验收。

## 贷款首期部署（2026-10-06）

已发布后端 `v0.1.0-loan-20261006`，公网静态前端随后切换为 `v0.1.0-loan-setup-r2-20261006`；应用机 `/opt/glorynavy/current` 与公网静态站点 `current` 已原子切换；Goose 由 69 升至 70。发布前在生产数据库快照库完成 `00070_loan.sql` 预检，正式切换前备份数据库与 `/etc/glorynavy/seat.env`，备份目录为 `/var/backups/glorynavy/before-v0.1.0-loan-20261006-20261006T071228Z/`。应用 `active/ready`、公网首页与 `/loans` 200、匿名模块目录 401、OpenResty `nginx -t` 均通过。

本次先部署贷款代码、迁移和静态资源，随后将生产 `MODULES` 加入 `loan` 并重启应用；登录成员现在可看到财务分组下的贷款入口。抵押折扣和真实 Tranquility 合同、分期还款、重复合同占用、担保/抵押和审批中心账号验收仍按待办继续跟进。启用开关前配置备份保存在应用机 `/var/backups/glorynavy/loan-module-enable-20261006T20261006T072436Z.env`。

## 预警消耗按监控星系归因（2026-10-06）

已发布 `v0.1.0-sentry-system-charge-20261006`。应用机和公网静态站点均已切换到该版本，应用 `active/ready`、公网首页 200、OpenResty `nginx -t` 通过。预警端按服务端确认的重叠监控星系拆分在线消耗，Seat 按星系聚合；无可靠归因证据的历史空 `system_id` 记录不猜测回填。无新增 Goose/River 迁移。

## 预警奖励说明卡片精简（2026-10-06）

已发布 `v0.1.0-sentry-reward-card-20261006`。应用机 `/opt/glorynavy/current` 与公网静态站点 `current` 已切换，应用 ready、公网首页 200、OpenResty `nginx -t` 通过。预警消费页移除独立的奖励说明卡片，奖励累计仍在消费统计指标和按星系合并的流水中；无 Goose/River 迁移。

## 监控奖励总额与记录口径对齐、离线 POS 燃料修复（2026-10-06）

已发布 `v0.1.0-sentry-reward-total-offline-pos-20261005`。奖励记录接口新增全量 `total_minor` 与 `total_count`，页面显示累计奖励和最近记录范围；不会再把最近 50 条展示记录误解为全部奖励。应用机已 ready，公网首页 200，建筑和奖励接口匿名 401。

建筑管理同批包含离线 POS 燃料修复：离线控制塔保留燃料库存但不显示耗尽时间，在线 POS 才显示按当前库存估算的到期时间。

## 生产发布分支门禁

生产包统一从远程 `main` 发布。其他分支必须先合并到 `main` 并推送，再执行构建；不能直接从功能分支或带未提交改动的工作区部署。发布前在仓库根目录执行：

```sh
npm run deploy:production:guard
node scripts/build-release.mjs v0.1.0
```

门禁会确认当前分支为 `main`、工作区干净，并且本地 `main` 与 `origin/main` 指向同一提交。构建产物的 `release.json` 会记录来源分支、提交和是否有未提交改动；应用机的激活脚本再次校验这些字段，非 `main` 或脏工作区产物会被拒绝。历史旧产物没有来源元数据时默认也拒绝，只有经过兼容性评估的应急回退才可显式设置 `ALLOW_LEGACY_RELEASE=1`。

本项目当前 GitHub HTTPS 推送使用代理 `http://10.233.53.209:7890`。PowerShell 推送前设置：

```powershell
$env:HTTPS_PROXY = "http://10.233.53.209:7890"
$env:HTTP_PROXY = "http://10.233.53.209:7890"
git push origin main
```

## 建筑管理只读模块（2026-10-05）

已发布应用为 `v0.1.0-structures-pos-fuel-fix-20261005-r6`，公网静态前端为 `v0.1.0-structures-state-label-20261005-r5`。后端在同步阶段使用 `esi-universe.read_structures.v1` 补查缺失的 Upwell 星系位置，再由本地 SDE 解析名称；页面仍只读取 Goose 69 的 `eve_structure_snapshots`。建筑管理页按 SDE 实际类型在军团内分组，顶部筛选使用统一下拉控件，燃料告警汇总支持一键只看 72 小时内到期建筑，卡片统一高度，类型分组与卡片置于同一容器，POS 燃料批量显示 SDE 物品名称，并按控制塔类型和燃料数量计算耗尽时间；离线 POS 保留燃料库存但不显示耗尽时间。与 Upwell 共用燃料到期条；类型徽标固定在卡片右上角，燃料到期使用高对比度状态条，建筑 ID从首屏移除，POS 服务保留可展开详情。服务 `active/ready`，公网首页 200、建筑接口匿名 401，OpenResty `nginx -t` 通过。生产 CEO `Nuter Zero` 的真实同步取得 20 座 Upwell、13 座 POS 及 13 条燃料明细，并已写入本地快照。详见[建筑本地快照发布记录](history/structures-local-snapshot-20261005.md)。回退前需先按 ESI 同步指南处理新 River 目标，不执行破坏性 Goose Down。

## 预警消费与监控奖励统一流水卡片（2026-10-04）

已发布 `v0.1.0-sentry-ledger-unified-r3-20261004`。后端按星系分组聚合交错的消费区间，并对同一星系不超过 300 秒的有效采样短间隔做展示合并；前端将同一星系连续、重叠或短间隔的预警扣费与监控奖励合并为一张流水卡片，分别显示负扣费、正奖励和状态。已释放/已退款消费保留正向返还显示，底层证据与币账仍逐笔保留；超过 300 秒的真实断档仍分开。应用机与公网静态前端均已切换，应用 active/ready、公网首页 200、匿名监控奖励接口 401，OpenResty `nginx -t` 通过。

## 预警记录按星系连续区间聚合（2026-10-04）

已发布 `v0.1.0-sentry-record-merge-20261004`。成员端展示合并同一星系跨客户端重连和旧 `legacy:` 标识的连续奖励区间；原始贡献记录、消费证据和果壳币账务保持逐条保存。应用与公网静态前端已切换，Goose 68、应用 active/ready、公网首页 200、匿名奖励接口 401 和 OpenResty `nginx -t` 通过。

## 预警密钥随时复制（2026-10-04）

已发布 `v0.1.0-sentry-key-copy-20261004`。服务端使用 `EVE_TOKEN_KEY` 派生密钥加密保存预警密钥，当前账号的受保护列表可在重新打开页面后返回完整内容，前端复制按钮不再依赖本地刷新后的临时状态；不会自动轮换。应用机与公网静态前端均已切换，Goose 68、应用 active/ready、公网首页 200、静态入口和 OpenResty `nginx -t` 通过。迁移前仅保存哈希的旧密钥无法恢复明文，需手动更新一次。

## Sentry 与批量结算统一发布（2026-10-04）

已将当前最新 Sentry 时间计费代码和批量结算修复合并为 `v0.1.0-sentry-batch-unified-20261004`，应用与公网前端统一切换到同一版本。收费只读取认证客户端在线区间，监控奖励只读取主节点贡献区间；事件、投递、ACK、授权释放和退款路径已清理。Goose/River 保持 66，无新增迁移；应用 ready、服务无重启、公网首页 200、受保护接口 401 和 OpenResty `nginx -t` 均通过。

## 预警时间计费清理发布（2026-10-04）

已发布 `v0.1.0-sentry-time-only-20261004`。应用机 `/opt/glorynavy/current` 与公网静态站点 `current` 均已原子切换；Goose 执行 `00067_sentry_system_metering`，按 `system_id` 保存每个星系的在线计费与主节点奖励贡献。事件、投递、ACK、授权释放和退款兼容路径及旧账表已移除。应用服务 `active/ready`，公网首页 200、预警消费接口匿名 401，OpenResty `nginx -t` 通过；旧版本仍保留在两台机器的 `releases/` 目录，可按本页回退流程切换。

## 批量结算修复版本恢复（2026-10-04）

并行 Sentry 发布曾将应用和公网前端覆盖到不含批量合同兼容逻辑的版本，#6 因此短暂回到“等待合并合同同步”。现已恢复应用 `v0.1.0-contract-batch-approval-history-20261003` 和前端 `v0.1.0-contract-batch-row-alignment-20261003`；#6 已重新识别游戏合同并进入“等待接收角色完成合同”，三条来源记录继续隐藏。生产健康检查、公网首页和匿名 401 均通过。

## 批量结算行列对齐修复（2026-10-03）

已仅更新公网静态前端至 `v0.1.0-contract-batch-row-alignment-20261003`。批量结算行按当前页签是否有选择列对齐表头，已处理页签的类型、申请人、项目、金额、状态和时间不再整体错位。后端、Goose、River 和应用服务未改；前端 `current`、首页 200、容器入口和 OpenResty `nginx -t` 通过。

## 批量结算来源去重修复（2026-10-03）

已仅更新公网静态前端至 `v0.1.0-contract-batch-queue-dedupe-20261003`。已进入批量结算的来源记录在批次生命周期内继续从普通“待发放”列表隐藏；批次移到“已处理”后不会再次显示三条来源记录。后端、Goose、River 和应用服务未改；前端 `current`、首页 200、容器入口和 OpenResty `nginx -t` 通过。

## 批量结算已发合同归入已处理（2026-10-03）

已发布 `v0.1.0-contract-batch-approval-history-20261003`。批量合同已同步但等待主角色接取时，后端返回 `delivery_status=awaiting_acceptance`，审批中心将其从“待发放”移到“已处理”，详情仍显示“等待领取合同”；合同未同步、内容不匹配或其他异常不改变原分类。批次 #6 生产数据仍为 `pending`、3 项待处理、完成数 0，三项错误均为“等待接收角色完成合同”。本轮无 Goose/River 新迁移，生产 Goose 66；应用 `active/ready`、`NRestarts=0`，公网前端 `current`、首页/登录 200、容器静态入口和 OpenResty `nginx -t` 检查通过。

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

当前应用和公网静态前端均为 `v0.1.0-loan-action-bar-20261007`，Goose 72。旧版产物与静态 assets 保留；本轮迁移及贷款模块开关边界见上文。详细验收及备份见[项目状态](project-status.md)，历次发布证据见[交付历史](history/project-status-through-2026-09-23.md)。联盟 PAP 每 30 分钟完整读取当前月快照后差量落库；支持手动与自动增量兑换，生产保留既有 manual 配置。

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

公网静态前端也使用同一产物来源门禁。在公网机解压后执行发布包内的 `deploy/activate-edge-release.sh`；脚本会校验 `release.json`、`SHA256SUMS` 和首页 200，再原子切换站点 `current`。不要直接手工切换来自其他分支的静态目录。

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
## 贷款合同处理人角色配置发布（2026-10-08）

已发布 `v0.1.0-loan-officer-character-20261008`。贷款管理者可在贷款设置中从已绑定 EVE 角色选择个人合同处理人；合同复制字段改为处理人角色 ID，入金、放款、还款和抵押核验按该角色执行，读取出借记录时自动推进已同步合同核验。无新增 Goose/River 迁移。

## 2026-10-17 游戏邮件待审批提醒发布

前后端 `v0.1.0-reminder-mail-20261017`，干净且已推送 main 来源 `1664eb4eb37a16f444f79899306c5a74d295903e`。发布包构建完成于 15:54 UTC，按现有 activate-release/activate-edge-release 流程启用。MODULES 增加 reminder，迁移 Goose 93；初始 sender_id=0、enabled=false、revision=1，尚无收件人或发送记录。额外邮件授权必须由管理员选择本人角色完成，当前未验证真实 ESI 发信和客户端送达。

备份目录 `/var/backups/glorynavy/`：原配置 `before-reminder-module-20261017T155434Z.env`；正式发布备份前缀 `before-v0.1.0-reminder-mail-20261017-20261017T155437Z`，包含 252205292 字节 dump、env 和 meta.json，权限均 0600。正式 env 已含新模块，若程序回退须恢复原配置或明确移除 reminder；恢复前按[提醒指南](integrations/reminder.zh-CN.md)停用发送、取消新类型未结束作业并保留回执。不得绕过迁移 Down 保护；发生提醒数据账号合并后不得回退缺少参与方的程序。

验证：reminder/approval 隔离 PostgreSQL 测试、EVE 邮件和旧登录/角色/配装写入回归、app/jobs/server 回归、相关 go vet/server 编译通过；补充扫描器缺少授权分支复验通过。前端类型/构建/lint、5 项词表单测、1280/375/320px 中英文 12 项浏览器流程通过并复验，截图已检查。首次测试发现窄屏溢出、缺失翻译和重复词条，修复后通过；窄屏仅表格横向滚动。记录状态未知不能重试、明确失败重试、版本冲突保留、授权不足与普通成员隐藏均已覆盖。

线上 active/running/ready、NRestarts=0，reminder 扫描 completed 2 条，发送记录 0；页面 200，所有新管理接口及额外授权入口匿名 401，HTML、新提醒包及主包逐字节哈希一致；OpenResty 配置与资源校验通过。QQ 保持 disabled/revision=6、容器 stopped，绑定和确认均 0，新旧 webhook 503。未恢复 QQ，也未代用户选择发件角色或开启邮件发送。发布脚本、检查与截图在忽略的 `.local/reminder-mail/`。



### 发信授权诊断发布（2026-10-18）

- `v0.1.0-reminder-auth-20261018` 来源干净且已推送的 `main` 提交 `1b58b57`，前后端同版；仅错误提示和有限回调日志，无新增迁移，Goose 93。
- 通过正式 activate-release / activate-edge-release 流程发布；数据库、环境和原版本元数据备份保存在 `/var/backups/glorynavy/before-v0.1.0-reminder-auth-20261018-<UTC 时间>.*`。服务 active/running/ready、NRestarts=0，OpenResty 语法检查、公网 200、HTML/资源哈希和管理接口匿名 401 通过。
- 发布后真实回调确认 `wrong_character`，EVE 返回身份不同于配置的发件角色；未绕过身份校验或手工增加 scope。随后同一角色真实授权成功，发信 scope 已保存、授权代次已推进；发送配置仍关闭、记录 0，客户端收信待验收。QQ 保持禁用/登出/无绑定，两类回调 503。
- 可回退前一版 `v0.1.0-reminder-mail-20261017` 的前后端以撤销诊断变更，保留 Goose 93；无需 Down。回退会失去更明确的授权错误提示和原因日志，不能解决角色不符。
