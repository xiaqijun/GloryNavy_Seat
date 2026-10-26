# 反馈撤销

日期：2026-10-26。

用户确认仅从未被采纳的待处理反馈可以由提交人直接撤销，已发奖励保留。实现使用 withdrawn 状态和 Goose 108；原账号锁、反馈行锁、revision 与 request_key 保证权限、竞争和重放边界。详情原操作区提供撤销确认，保留记录并可填写原因重新提交。重提使用独立 returned 事件引用，撤销不再是管理员待办。

影响检查：statusOK、状态校验和提醒方法的可解析 upstream 风险为 LOW；较新的 validateTransition/FeedbackWorkflow 未纳入旧索引（落后 24 次提交），已按源码检查 Service.Reply 和 Thread 调用。索引刷新长时间无进展而停止，不把未知调用当作未使用。无新跨模块 store、权限或币账路径。

先在未修复源码执行新增策略用例，拥有者撤销失败；修复后在独立 PostgreSQL 容器执行反馈及宿主 race、vet 和全部命令构建。检查覆盖本人/他人/管理员、已采纳历史、并发采纳和奖励、旧请求重放、重新提交提醒、撤销历史阻止 Down；生产数据不用于写入测试。英语确认文案初次与既有凭据吊销键重名，类型/浏览器检查发现后改用独立的“确认撤销反馈”，不改凭据吊销文案。

最终核验：反馈模块/私有 store 及宿主集成的定向 `go test -race -count=1`、`go vet`、全部命令构建和 locale race 通过。TypeScript、定向 oxlint、后端词表检查、Vite 构建及首页预渲染通过；最终构建的 42 项浏览器回归全部通过（中英文，1280/375/320px，浅色/暗色），覆盖撤销确认、取消不发送、忙碌保护、权限、采纳历史、重新提交和原有完整流程。桌面与窄屏截图见[撤销确认](../ui/reviews/feedback-withdrawal/desktop-confirm-withdraw.png)、[暗色撤销后](../ui/reviews/feedback-withdrawal/narrow-withdrawn.png)。已人工检查截图，无横向溢出。

发布结果待完成后补充。真实成员操作与游戏邮件仍按 FEEDBACK-FLOW-01 验收，不使用生产反馈写入作为测试。
