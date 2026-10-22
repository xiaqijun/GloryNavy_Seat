# 审批中心旗舰金额

2026-10-23：待审批的超级航母申请尚未生成 award_minor，审批列表只读取该字段导致金额显示为横线。详情页已有 capitalAmount，按冻结购舰金额和比例计算。

精简列表原本也省略了购舰证据和自定义比例。福利来源投影使用批准时的 capitalAward 计算 `payload.estimated_award_minor`，前端在 submitted / information / external 阶段显示“预计补贴”；批准后使用既有核准金额。旧查询完整载荷兼容详情页 capitalAmount。超级航母和泰坦共用修复，缺少购舰证据不推算，沿用整数最小货币单位与向下取整。新增可选只读载荷字段，无业务写入或迁移。

GitNexus upstream：ApprovalValue 仅一个直接调用者 Workspace，LOW；索引落后 8 个提交，期间快递／LP／文档变更未改审批金额逻辑，手工核对当前源码。新增导入不修改 capitalAmount。

验证：应用机隔离 PostgreSQL 下福利模块 6 组 Approval 测试（含 9 项旗舰金额场景）race 通过，vet 通过；前端类型、定向 lint、5 项单元测试及生产构建通过；中英文桌面／375px 手机 4 项金额场景检查与 2 项既有审批列表回归通过，并检查手机截图。模拟接口使用精简投影字段，未假设列表携带完整购舰证据。首次并行浏览器运行因首次加载超时失败，降低并发后最终 6 项全部通过。测试不代表真实游戏合同交付。

前后端 `v0.1.0-approval-capital-amount-20261023` 已从干净且推送的 main `cfd65076dec2ceac64f5ed78b9ca77416a621b72` 发布。后端所有发布程序构建通过，构建前逐文件核对测试源码；前端发布包与上述测试构建逐文件一致。服务器备份后切换，健康 ready、OpenResty 检查及线上资源哈希通过；审批 API 匿名 401。Goose 103，无数据迁移，原订单、核准金额、合同和币账不改写。

线上只读复核 #125：submitted / version 1 / award_minor 0 保留；申请快照 base_minor 3,800,000,000,000、rate_bps 1000，列表投影新增 estimated_award_minor 380,000,000,000，即 3.8B ISK，fresh 且 source_version 1。真实登录浏览器未复测，不把只读投影与静态资源核验当作登录态端到端验收。前后端前版保留供回退。
