# 建筑管理

建筑管理首期是只读总览，路径为 `/structures`，接口为 `GET /api/v1/structures/structures`。接口按当前登录账号的有效绑定角色和 `corporation.structure` 对象权限读取军团建筑；可用 `corporation_id` 指定军团。

数据来自 ESI 的军团 Upwell 建筑和 POS 端点，并复用 ESI 的共享缓存与限流。Upwell 建筑展示状态、服务、profile、燃料到期时间；POS 读取母星基地详情中的燃料类型和数量，详情请求按 ESI 要求带上 POS 所在星系 `system_id`。`observed_at` 表示本次 ESI 响应的观测时间，不能当作实时游戏状态。

本期不提供租用、收费、合同、Access List/Profile 编辑、个人进入权限控制、POS 密码或其他游戏内写操作。游戏内 ACL、Profile 和角色权限仍在 EVE 客户端的 Structure Browser 中维护。ESI 授权需要军团建筑读取 scope；角色未完成授权、角色不再属于目标军团或 ESI 返回不可用时，接口不会伪造空数据。

## 权限

`corporation.structure` 已登记到可配置权限目录。CEO/Director 的游戏职务或管理员/站点 RBAC 授权可通过对象级检查；前端导航不构成授权。模块权限 `structures.self` 只表示已登录，目标军团权限仍由后端逐次复核。

## 相关 ESI scope

- `esi-corporations.read_structures.v1`：Upwell 建筑列表。
- `esi-corporations.read_starbases.v1`：POS 列表和燃料详情。
- `esi-universe.read_structures.v1`：后续解析建筑静态名称时使用；当前接口返回游戏 ID。
