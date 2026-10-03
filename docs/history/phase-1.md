# 第一阶段：工程底座

> 历史阶段 / 方案记录：保留当时决策和测试结果，本文不是当前能力清单或最新部署指令。当前状态见[项目状态](../project-status.md)，尚未完成事项见[待办](../backlog.md)。

> 历史交付记录：以下范围、迁移版本与测试结果对应当时阶段。当前状态见[项目状态](../project-status.md)，当前运行步骤见[开发指南](../development.md)，后续变化见[变更记录](../../CHANGELOG.md)。

完成日期：2026-09-13。目标是建立可以实际运行、检查和继续开发的平台基础。

本文保留阶段历史；后续身份登录实现和当前验证边界见 [EVE 登录交付记录](phase-2-login.md)。

## 已交付

- Go 1.27.1 + chi HTTP 服务；环境配置校验、JSON 日志、服务端请求编号、统一 JSON 错误、超时、优雅退出。
- PostgreSQL + pgx 连接池；Goose 独立迁移命令；sqlc 1.31.1 查询生成配置及生成代码。API 不在启动时自行修改数据库。
- 三个接口：进程存活、数据库就绪、工作台系统状态；数据库未迁移、连接失败或基础版本不受支持时返回 503。
- React + TypeScript + Vite；React Router、TanStack Query、shadcn/ui Button/Card 和 Tailwind 主题。
- Corporate Clean 中文工作台与系统状态页；桌面侧栏、手机导航、真实连接状态、加载/错误/重试、键盘跳转及减少动效支持。
- OpenAPI 3.0 契约、开发环境示例、PostgreSQL 18.6 开发 Compose、跨平台 Node 启动脚本及 GitHub Actions 检查流程。

依赖具体版本以 `go.mod`、`web/package.json` 和 `web/package-lock.json` 为准。未提前安装业务表格、图表、表单、任务队列等尚未使用的库。

## 本机验证

| 检查 | 结果 |
| --- | --- |
| Go vet 与 Go 测试 | 通过；覆盖配置边界、健康检查隔离、错误与日志脱敏、panic 处理 |
| PostgreSQL 集成测试 | 通过；在随机独立 schema 中执行迁移、重复迁移、回滚、重新迁移及不兼容版本检查 |
| sqlc 重新生成 | 通过；生成前后文件哈希一致 |
| 前端 lint、TypeScript 与生产构建 | 通过 |
| Vitest | 3 项通过，覆盖异常响应与请求编号 |
| Playwright | 6 项通过，桌面/手机均覆盖真实 API 及导航、503 后刷新恢复、键盘入口和减少动效 |
| 页面截图检查 | 已检查桌面 1440px、手机 390px；修复路由切换后的滚动位置与键盘焦点 |
| Linux amd64 交叉编译 | API 和迁移工具均成功；未在 Linux 运行 |

验证使用独立 PostgreSQL **16.14** 实例，不是目标 PostgreSQL 18；本机 Chrome 152 执行 Playwright，默认配套 Chromium 下载受网络影响未完成。可通过 `PLAYWRIGHT_CHROMIUM_EXECUTABLE` 指向已有 Chromium 系浏览器；CI 使用 Playwright 安装的版本。

最终前端资源约为 JS 309 kB（gzip 99 kB）、CSS 19 kB（gzip 5 kB）。这是基础页面构建体积，不是生产内存占用或加载性能压测结论。

## 验证边界

- 本机没有 Docker：Compose 已编写，尚未实机执行。目标版本依据 [PostgreSQL 18.6 官方发布说明](https://www.postgresql.org/docs/release/18.6/)。
- GitHub Actions 已编写，尚未在远程仓库运行；PostgreSQL 18.6 与 Go race 检查由该流程覆盖，不能将配置存在当作运行通过。
- 暂无 EVE SSO 登录、角色/账号模型、RBAC、ESI 调度、River、SDE 导入或业务报表。
- 生产 Caddy/容器部署、密钥管理、备份恢复和 4C4G 压测尚未执行。
- UI 已做基础键盘、布局与动效检查，尚未完成屏幕阅读器和全套无障碍审计。

## 开发入口与下一步

当前工作机可执行 `npm run dev:external`；全新开发环境按 [README](../../README.md) 使用 Docker 路径启动。`.env`、`.local`、`.tools`、编译产物和数据库文件均已列入忽略规则。

第二阶段按已确认需求打通身份链路：EVE SSO → 平台账号与角色绑定 → 会话 → 手填 QQ 号与 KOOK 昵称 → 基础权限；社区确认单独维护，QQ 机器人后续接入。接入前需要开发者应用的 Client ID、Client Secret 和回调地址，配置到本地环境，不能写入仓库。随后再实现 ESI 同步与 SDE 导入；接入约定沿用 [中英文接入文档](../integrations/README.md)。

## 2026-09-14 架构调整

系统状态已迁入 `internal/modules/system`，SQL 私有化；新增 Go 模块注册表和前端模块目录，页面与导航由同一声明生成。受保护接口缺少宿主鉴权时启动失败；当前只注册匿名运维接口，没有提前实现登录。

当时暂停的认证 SQL 曾隔离为草稿，已于 2026-09-23 文档清理时删除；本段记录当时 foundation v1，不是当前 Goose 版本。完整约定见 [模块架构](../architecture.md) 与 [开发约定](../module-development.md)。本段之后的验证记录以实际本机结果补充，上述首阶段测试数量和构建体积为历史记录。

本次架构调整验证：

- `npm run check` 通过：Go vet、Go 单元测试及 PostgreSQL 16.14 集成测试、前端 lint、8 项 Vitest、TypeScript 和生产构建。
- 注册表测试覆盖依赖排序、停用模块、缺失依赖、循环/版本冲突、路由冲突及宿主权限拒绝；宿主组装测试验证模块目录和存活检查无需数据库。
- sqlc 重新生成前后模块 store 的 Go 文件哈希一致；开发启动确认没有待执行迁移，当前版本为 1。
- Playwright 桌面/手机共 8 项通过，新增模块目录不可用后的恢复检查。首次 3 worker 执行时 Vite 所在 npm 进程以退出码 3221226505 异常退出，导致后续连接中断；重启后以 `--workers=1` 完整通过，异常退出根因尚未确定，不据此认定并行运行稳定。
- 检查 1440px、375px、812px 横屏的工作台与系统状态页，无横向溢出和页面脚本错误；已查看桌面与 375px 系统状态截图，保留原有布局及图标操作。
- 本次仍未运行 Docker、目标 PostgreSQL 18、远程 CI 或生产负载验证。
