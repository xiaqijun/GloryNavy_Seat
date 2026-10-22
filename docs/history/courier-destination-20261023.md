# 快递终点名称修正

用户反馈“Jita · JF bulkhead”地址有误，随后确认官方仅显示 Jita，且没有运输类型选项。应用机通过既有代理只读核对 HA Logistics 原始 API：路线 272 为 S-KSWL - Glory Navy Staging Point → Jita，end_location_id=30000142，transport_type=JF bulkhead；数据同步正确，本站展示误将运输类型拼进地点。

## 最终修改

终点下拉只显示服务方 end 原文，不增加运输类型字段或控件，不把 Jita 推断为吉他 4-4。仍按 route ID 选取费用、期限和容量，复制仍使用原始 end。仅一行前端逻辑变化；没有后端、ESI、数据库、费用或合同规则变动。

GitNexus Workspace upstream 影响 LOW，仅直接调用者 CourierPage，无额外流程。索引落后 6 个提交，这些提交未改快递页面；已尝试刷新，长时间无进展后停止刷新，使用现有索引加手工差异核对。提交前另运行 detect_changes。

## 验证

- TypeScript、Vite/公开首页构建及定向 lint 通过，4 项现有快递单元测试通过。
- 现有快递 Playwright 24 项通过：中英文，1440/375/320px，报价、复制、估价失效和错误重试。
- 基于上述官方路线数据的 4 项定向浏览器检查通过：中英文、1280/375px，终点与选项仅 Jita、没有新增运输类型控件、费用 25,000,000 ISK、复制地址严格等于 Jita；桌面及移动端截图已检查。
- 浏览器验证使用模拟会话/接口的正式构建，不是已登录生产验收。上游路线数据另以真实只读请求核对。

## 发布

前端发布待完成，后端保持 v0.1.0-courier-appraisal-cache-20261023、Goose 103。
