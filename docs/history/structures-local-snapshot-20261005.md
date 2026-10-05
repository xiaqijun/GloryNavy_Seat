# 建筑本地快照发布记录（2026-10-05）

生产版本：`v0.1.0-structures-local-snapshot-20261005-r1`。应用机 `10.233.53.209` 与公网静态站点已切换，服务 `active/ready`，Goose 69 已完成，公网首页 200，匿名建筑接口 401，OpenResty `nginx -t` 通过。

本次新增 `eve_structure_snapshots` 表和 `corporation_structures` River 资源。生产 CEO `Nuter Zero` 的同步任务成功写入本地快照；快照记录显示军团 `98530802` 每份 payload 含 33 条建筑（20 座 Upwell、13 座 POS），其中 POS 燃料明细随快照保存。页面读取这些快照，不在用户请求中调用 ESI。

旧的 [建筑 ESI 验收记录](structures-esi-acceptance-20261005.md) 保留为上游端点和响应解析证据；本记录补充数据库落库与发布验证。
