# 审批来源接入模板

本文是新增审批流接入审批中心的最小模板。来源模块继续拥有原始单据、规则、详情和写操作；审批中心只保存列表摘要和投影状态。

## 注册清单

在宿主的审批组合处注册一个 `reviewqueue.Source`：

| 字段 | 要求 |
| --- | --- |
| `ID` | 稳定的小写来源标识；不能与现有来源重复 |
| `Capabilities` | 明确批准、驳回、取消审核、发放、筛选和 `DetailKind` |
| `Access` | 返回当前操作者的来源范围；拒绝时不能泄露对象 |
| `IndexAccess` | 只有索引范围与普通上下文不同才提供 |
| `Snapshot` | 返回列表摘要，不包含令牌、合同全文、账务明细或完整审计 |
| `Query` | 仅作为索引未启用或回退时的旧聚合适配器；新来源可使用 `IndexOnly` |
| `Decorate` | 只补充操作者相关动作和展示字段，不能写库或逐行访问 SDE |

宿主启动会调用 `reviewqueue.ValidateSources`，缺少访问、快照、旧查询（除非 `IndexOnly`）或详情能力时直接失败。

## 摘要与事件样例

来源快照转换为 `reviewqueue.Item` 时只填列表字段：

```json
{
  "source": "example",
  "id": "42",
  "version": "7",
  "account_id": "account-uuid",
  "corporation_id": "123",
  "kind": "example",
  "state": "submitted",
  "status": "pending",
  "title": "列表摘要标题",
  "reference": "EX-20261010-42",
  "recipient": "9001",
  "amount_minor": 1000000,
  "unit": "isk",
  "time": "2026-10-10T00:00:00Z"
}
```

若来源接入事件或 outbox，事件只需携带最小坐标与摘要：

```json
{
  "source": "example",
  "source_id": "42",
  "source_version": "7",
  "event": "updated",
  "occurred_at": "2026-10-10T00:00:00Z",
  "summary": { "state": "submitted", "status": "pending", "amount_minor": 1000000 }
}
```

旧版本事件必须被版本保护拒绝；周期快照对账负责补齐丢失事件。事件中禁止放令牌、合同完整内容、奖励物品全集和币账流水。

## 权限清单

- 在 `ManageableCatalog` 登记来源真正需要的管理能力；`approval.self` 只代表会话能访问审批 API，不是业务管理权限。
- `Access`、`IndexAccess`、详情和决定接口都检查操作者、对象归属、有效绑定、军团范围和自审限制。
- 普通成员不可因前端菜单可见性获得审批权限；管理员也不能审批本人或审核自己的取消。
- 撤销权限、解绑账号或军团范围变化后，后续详情和写操作必须立即拒绝。

## 测试夹具与验收

使用 `internal/platform/reviewqueue/testfixture` 的内存来源夹具覆盖：

1. 能力和来源 ID 启动校验；
2. 索引列表不调用旧 `Query`；
3. 版本较旧的快照不能覆盖新摘要；
4. 权限撤销、本人记录、取消待审和历史分页；
5. 来源失败只标记该来源，不把其他来源计数清零；
6. 详情和决定仍由来源模块重新鉴权。

新增来源只需注册适配器、实现快照/详情/决定并复用该夹具，不修改审批中心的分页、计数或主查询逻辑。
