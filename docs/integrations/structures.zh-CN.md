# 建筑管理

建筑管理首期是只读总览，路径为 `/structures`，接口为 `GET /api/v1/structures/structures`。接口按当前登录账号的有效绑定角色和 `corporation.structure` 对象权限读取军团建筑；可用 `corporation_id` 指定军团。

页面只读取本地 `eve_structure_snapshots` 快照，不在用户请求中直连 ESI。River 的 `corporation_structures` 任务按授权角色从 ESI 更新快照，并复用 ESI 的共享缓存与限流；快照包含 Upwell 建筑状态、服务、profile、燃料到期时间，以及 POS 的燃料类型和数量。返回投影在页面请求时批量通过本地 SDE 补充建筑类型与 POS 燃料物品名称，并根据控制塔类型、燃料块和高安 Charter 数量计算 POS 燃料耗尽时间，统一写入 `fuel_expires`，不逐项请求 ESI。`observed_at` 表示最近一次成功同步的观测时间，不能当作实时游戏状态。

本期不提供租用、收费、合同、Access List/Profile 编辑、个人进入权限控制、POS 密码或其他游戏内写操作。游戏内 ACL、Profile 和角色权限仍在 EVE 客户端的 Structure Browser 中维护。ESI 授权需要军团建筑读取 scope；角色未完成授权、角色不再属于目标军团或 ESI 返回不可用时，接口不会伪造空数据。

## 权限

`corporation.structure` 已登记到可配置权限目录。CEO/Director 的游戏职务或管理员/站点 RBAC 授权可通过对象级检查；站点管理员读取时会选择当前有效的 CEO/Director 授权作为数据源，普通成员仍只使用自己的有效绑定角色。前端导航不构成授权。模块权限 `structures.self` 只表示已登录，目标军团权限仍由后端逐次复核。

## 相关 ESI scope

- `esi-corporations.read_structures.v1`：Upwell 建筑列表。
- `esi-corporations.read_starbases.v1`：POS 列表和燃料详情。
- `esi-universe.read_structures.v1`：当军团建筑列表没有返回星系 ID 时，后台按建筑 ID补查位置；成功后再通过本地 SDE 显示星系名称。若角色没有该 scope 或建筑 ACL 不允许解析，则保留“未知星系”，不伪造位置。
