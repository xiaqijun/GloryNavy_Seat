# 本地开发与验证

## 模块启用提示（历史引入版本）

当前迁移统一运行 `npm run db:migrate` 到仓库最新版本（现为 Goose 46，同时维护 River），不要把下列功能首次引入版本当作升级终点。`MODULES` 按实际需要追加并保留依赖；示例未默认启用 welfare，使用福利时显式添加。

### PAP 要求配置（首次 Goose 43）

新增 attendance 私有全局配置及审计表；先备份数据库/配置，再运行 npm run db:migrate 并重启 API。新界面需要新接口，不更改会话或现有 PAP/币流水。规则和验证范围见 [考勤指南](integrations/attendance.zh-CN.md)。生产版本仍以 [项目状态](project-status.md) 为准。


## 本地审批中心

MODULES 增加 approval 即启用统一管理入口 /approvals，并保留需要的 welfare/exchange 来源模块。无额外迁移、scope 或会话清理。后端构建并重启、前端构建后生效；Vite preview 使用 web/dist。未启用 approval 时原管理入口保留作为兼容回退。见[审批中心指南](integrations/approval.zh-CN.md)。

本地物品估价模块：在原 MODULES 清单添加 market（依赖 identity/eve/access），备份后执行 npm run db:migrate（Goose 36），重启后在“财务与工具 → 物品估价”使用。默认统一比例 100%，仅站点管理员可配置；不触发福利付款。见[指南](integrations/market.zh-CN.md)。

## 本地 ISK 钱包

备份后 `npm run db:migrate` 升级 Goose 33/34，向现有 `MODULES` 追加 `wallet` 并重启 API、重新构建前端。使用 [钱包指南](integrations/wallet.zh-CN.md)。本机独立 PostgreSQL 的启动参数应明确传入 `-o "-h 127.0.0.1 -p 55432"`，避免默认端口与系统其他实例冲突。


本文对应根目录 `package.json`、`scripts/tasks.mjs` 和 `.env.example`，用于开发环境；Linux 构建、systemd、独立数据库与反代证书流程见[生产部署](deployment.md)。项目功能状态见[项目状态](project-status.md)。

## 本地开发

福利首版需要先备份本地数据库、运行 `npm run db:migrate` 升级 Goose 31，再向现有 `MODULES` 追加 `welfare`（保留 exchange 等依赖）并重启 API。入口 `/welfare`；初始规则与资格为空，管理员需明确配置。使用和回退约束见[福利指南](integrations/welfare.zh-CN.md)，本地启用不等于生产发布。

准备 Go 1.27.1、Node.js 24.15.0+、Docker Compose v2。仓库提供的数据库仅供本地开发。

```sh
npm run setup
npm run dev
```

`setup` 安装依赖并在缺失时创建 `.env`。`dev` 启动 PostgreSQL 18.6 容器、执行迁移、编译并启动 Go API 和 Vite。浏览器打开 <http://127.0.0.1:5173>。前端修改即时更新；Go 修改后重启开发命令。Ctrl+C 停止 API 和前端，数据库容器保留；使用 `docker compose -f compose.dev.yaml stop` 停止数据库。

没有 Docker 时，可使用已有 PostgreSQL：创建独立数据库，在 `.env` 中设置 `DATABASE_URL`，执行 `npm run dev:external`。当前工作机使用独立的 `.local/pgdata` PostgreSQL 16 实例（端口 55432），没有修改已有数据库。`.tools/go` 和 `.tools/sqlc` 为本机工具，均不会纳入版本管理。

本机重启独立数据库（PowerShell）：

```powershell
& 'C:\Program Files\PostgreSQL\16\bin\pg_ctl.exe' -D "$PWD\.local\pgdata" -l "$PWD\.local\postgres.log" -o "-h 127.0.0.1 -p 55432" start
npm run dev:external
```

这个本机命令依赖已初始化的 `.local/pgdata`，不适用于全新克隆。不要同时启动占用 55432 的本地实例和 Compose 数据库。

## 本地页面失去响应诊断

Vite 开发环境自动加载交互诊断；生产构建不包含该模块和 watchdog Worker。诊断不会阻止应用启动，也不自动解除点击锁、关闭弹窗或改动业务数据。热更新卸载时清理定时器、监听器和 Worker。

- 可见页面的 `body` 持续禁止点击，且没有可见弹窗/下拉超过 2 秒时，记录 `orphan_pointer_lock`。
- 独立 Worker 超过 5 秒未收到可见页面心跳时，输出 `main_thread_stall`；主线程恢复后保存记录。调试器暂停、设备休眠或浏览器调度也可能导致心跳延迟，因此它是排查证据，不等于已确定代码死循环。
- 前端异常只保存 `render_loop`、`script_error` 或 `unhandled_rejection` 分类，不保存原始错误文本、堆栈、账号、令牌、请求参数或响应内容。
- 控制台前缀为 `[GloryNavy interaction]` / `[GloryNavy interaction watchdog]`；最近 40 条异常记录保存在当前标签的 `sessionStorage` 键 `gnv:interaction-diagnostics`，刷新后会回显最近 5 条。仅保存白名单页面路径（不含查询参数）、时间、分类及阻塞时长。标签关闭后记录随会话存储结束；浏览器进程完全崩溃时不能保证保留。

F12 无法读取时，2026-09-16 新增本机诊断接收器：仅 `vite serve` 在回环地址提供 `GET/POST /__debug/interaction`，不注册到 Go、生产构建或 preview。页面/Worker 将固定的异常分类、白名单页面路径、主线程/Worker 来源、浏览器类别及每次加载随机生成的标签标识送到同源本地服务，不携带 Cookie、账号、原始 URL、错误文本或堆栈。启动及菜单点击也记录，以确认故障标签是否加载了诊断。Worker 可在主线程阻塞时独立发送，但整个渲染进程停顿、网络线程故障或调试器暂停仍可能影响记录；不能把心跳超时直接认定为应用死循环。

接收器校验回环来源、Host 和写入 Origin，拒绝跨源访问，最多读取 1 KiB 请求，丢弃额外字段，仅保留最新 80 条内存记录，重启 Vite 清空。不会发送到生产或第三方。故障复现前刷新本地页面；复现后在 PowerShell 读取：

```powershell
Invoke-RestMethod http://127.0.0.1:5173/__debug/interaction | ConvertTo-Json -Depth 4
```

先用 `ready` 的浏览器类别及随机标签标识区分原 Edge 标签与自动化测试；自动化注入的阻塞记录不可作为真实问题复现证据。

每 5 秒额外更新两条最新 `heartbeat`：主线程记录可见性、body 是否禁止点击、可见浮层数量及当前菜单中心是否命中自身；Worker 记录上次主线程心跳的间隔。每个随机标签/线程仅保留最新心跳，不逐次累积。不记录 DOM 文案、选择器内容或页面 HTML。复现后停留在故障标签约 10 秒再读取，以区分浏览器后台节流、主线程停止与输入/绘制异常。

- 白名单包含 `/welfare` 和 `/losses`。福利/损失连续切换回归位于 `web/e2e/welfare.spec.ts` 的“反复切换福利与损失页面并关闭下拉后仍可操作”；独立 Edge 无扩展夹具测试通过，不代表原用户配置中的卡死已修复。

有窗口、关闭 GPU 的独立 Edge 对照可在 `web` 目录执行以下命令（路径按本机安装位置替换），不改日常浏览器设置。Playwright 仍使用临时配置及自动化默认参数，测试数据为夹具，不能视为与用户现场完全相同。

```powershell
$env:PLAYWRIGHT_CHROMIUM_EXECUTABLE = 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'
$env:PLAYWRIGHT_DISABLE_GPU = '1'
npm run test:e2e -- exchange.spec.ts welfare.spec.ts --project=desktop --workers=1 --headed --grep '现场速度|反复切换福利'
Remove-Item Env:PLAYWRIGHT_DISABLE_GPU
Remove-Item Env:PLAYWRIGHT_CHROMIUM_EXECUTABLE
```

2026-09-16 焦点对照未改善用户原 Edge 的卡死，已撤回该调整和专项测试，恢复原导航焦点行为。

更接近连续鼠标输入的独立测试（在 `web` 目录，先设置上述 Edge 路径及 GPU 环境变量）：

```powershell
npm run test:e2e -- exchange.spec.ts --config playwright.edge-repro.config.ts --grep '连续鼠标切换'
```

该配置关闭 trace，去掉 Playwright 用于关闭 hang monitor、IPC flood protection 和三项后台节流的启动参数，保留浏览器自身机制；使用原生浏览器 UA。测试先定位桌面菜单，再按固定坐标进行 100 轮鼠标切换，每次间隔 100ms，不在每次点击前检查元素稳定性；末尾检查页面和表单仍可操作，并捕获脚本错误及导航 Intervention。依然采用临时浏览器配置、其余 Playwright 参数与业务夹具（配装列表为空），不是原 Edge 的等价复现。此项不用于手机导航布局。

若现场在编译版本仍复现，且 DevTools 在卡住前可打开：在原 Edge 的 Performance 面板开始录制，保持 CPU/网络不限速，操作菜单至卡住后约 5 秒停止并保存 profile。仅录制导航操作，文件留在 `.local/perf/`；该目录被 Git 忽略，不提交原始诊断文件。记录停止是否响应；无法停止时不要用空文件认定无异常。分析 Main 脚本、样式/布局与帧活动，原始请求 URL/业务内容不抄入项目文档。参见[微软性能录制指南](https://learn.microsoft.com/en-us/microsoft-edge/devtools/performance/)。

复现时尽量保留失去响应的标签，先读取控制台证据；不要将手动注入的点击锁或阻塞测试当作用户问题已复现。专项验证：`npm --prefix web run test:e2e -- interaction-diagnostics.spec.ts`；正常嵌套浮层的误报检查在 `skills.spec.ts` 中。

### 编译前端的本地对照

用于判断故障是否依赖开发运行环境。先保留故障诊断，再停止占用 5173 的本地 Vite 前端进程，保留 API 和数据库。在项目根目录执行：

```powershell
npm --prefix web run build
if ($LASTEXITCODE -ne 0) { throw '前端构建失败' }
npm --prefix web run preview:local
```

访问原 `http://127.0.0.1:5173`，刷新以加载编译资源。`/api` 默认代理本地 `127.0.0.1:8080`，非默认 API 地址需设置 `API_PROXY_TARGET`。同源地址与 SSO 回调保持一致。该模式无 HMR、React 开发检查和交互诊断 Worker；修改源代码后须重新构建。`/__debug/interaction` 此时不提供诊断 JSON，不能用缺少心跳认定页面停止。

恢复开发时停止 preview，再执行 `npm --prefix web run dev`；两者不能同时占用 5173。不用于正式生产托管，不执行迁移或部署。诊断专项依赖 Vite 开发接收器，须恢复 dev 后执行；普通业务夹具导航测试可用于 preview 对照。

## 常用命令

| 命令 | 功能 |
| --- | --- |
| `npm run auth:key` | 在本机 .env 生成缺失的 EVE 令牌加密密钥 |
| `npm run access:admin -- --character ID` | 为已登录的角色对应账号设置本站管理员 |
| `npm run sde:import -- --latest` | 检查官方 Tranquility 新版本并按需导入物品中英文名称，见[SDE 运行指南](integrations/sde-names.zh-CN.md) |
| `npm run sde:import -- --status` | 查看活动 SDE、最近检查结果及下次检查时间 |
| `npm run sde:import -- --resume` | 解除手动回退后的版本固定，使下次自动检查到期 |
| `npm run db:migrate` | 依次应用 Goose 与 River 自有迁移，可重复执行 |
| `npm run db:generate` | 使用 sqlc 1.31.1 重新生成查询代码 |
| `npm run check` | Go vet、Go 测试、前端 lint、单元测试与生产构建 |
| `npm --prefix web run test:e2e` | 桌面与手机浏览器测试（先迁移数据库） |
| `npm run build` | 生成 `bin/server`（Windows 为 `.exe`）与 `web/dist` |

首次浏览器测试前在 `web` 目录运行 `npx playwright install chromium`。下载受限时，可设置 `PLAYWRIGHT_CHROMIUM_EXECUTABLE` 为已安装 Chrome 的绝对路径。设置 `TEST_DATABASE_URL` 后，Go 测试会在该数据库的随机独立 schema 内执行迁移、重复迁移、回滚与就绪检查，完成后删除该测试 schema；不设置时明确跳过集成测试。请指向开发/测试数据库。

如 Go 官方代理在当前网络不可达，可为当前终端设置 `GOPROXY=https://goproxy.cn,direct`，保留默认校验机制。项目不修改全局 Go 配置。

## 配置说明

配置样例见 [`.env.example`](../.env.example)，脚本读取本机 `.env`，终端中已设置的同名环境变量优先生效。修改 Go 代码或启动配置后重启开发命令。

| 配置 | 当前行为 |
| --- | --- |
| `DATABASE_URL` | 必填，连接独立开发数据库；示例账号仅供本地使用 |
| `HTTP_ADDR` | API 默认 `127.0.0.1:8080` |
| `PUBLIC_ORIGIN` | 默认 `http://127.0.0.1:5173`，不带路径或尾斜杠；本地回环允许 HTTP，其他环境要求 HTTPS |
| `EVE_ENVIRONMENT` | 当前仅支持 `tranquility` |
| `MODULES` | 默认 `system,identity,eve,access,community,attendance,sentry`；system 必需，依赖必须完整 |
| `EVE_CLIENT_ID` / `EVE_CLIENT_SECRET` | 两项都为空时登录不可用；启用时必须成对提供，仅在后端使用 |
| `EVE_TOKEN_KEY` | 启用 eve 且配置 EVE 登录时需要 Base64 编码的 32 字节密钥；只在缺失时生成，升级时保留 |
| `ESI_USER_AGENT` | 默认 GloryNavy/0.1.0；部署前建议补充真实运维联系方式，只接受最多 256 个可打印 ASCII 字符 |
| `DB_MAX_CONNS` | 默认 10，配置范围 1–100；默认值不是容量验收结论 |
| `LOG_LEVEL` | 默认 `info`，由 Go slog 校验 |
| `API_PROXY_TARGET` | Vite 代理目标；开发脚本在未指定时按 `HTTP_ADDR` 生成 |
| `TEST_DATABASE_URL` | Go 数据库集成测试使用的独立开发/测试库 |
| `PLAYWRIGHT_CHROMIUM_EXECUTABLE` | 浏览器测试使用已安装 Chrome 时提供绝对路径 |
| `SENTRY_INTEGRATION_URL` / `SENTRY_INTEGRATION_TOKEN` | 可选的后端到 EVE Sentry 密钥投影地址与服务凭据；必须成对配置，HTTPS origin（本地回环可用 HTTP），Token 至少 32 字符；不下发前端 |
| `SENTRY_ALERT_CONSUMPTION_ENABLED` | 默认为 `false`；仅在 Sentry 时间区间、生产费率、使用证据和 exchange 对账验收后设为 `true`，且要求 `MODULES` 同时包含 `sentry`、`exchange`、`eve` |
| `SENTRY_ALERT_PRICE_VERSION` | 启用预警收费时必填的冻结费率版本；无默认值 |
| `SENTRY_ALERT_UNIT_SECONDS` / `SENTRY_ALERT_UNIT_PRICE_MINOR` | 启用预警收费时必填的时间单位秒数与每单位果壳币最小单位价格；无默认值 |
| `SENTRY_ALERT_MAX_GRANT_SECONDS` / `SENTRY_ALERT_GRANT_TTL` | 启用预警收费时必填的单次授权上限与授权有效期；无默认值 |
| `QQ_BOT_APP_ID` / `QQ_BOT_APP_SECRET` / `QQ_BOT_API_BASE` | 官方 QQ Bot 的迁移兜底配置；管理员首次在 `/account` 保存后由数据库设置接管，Secret 只写入并加密保存 |
| `QQ_BOT_GROUP_OPENIDS` | 官方 QQ Bot 自动审批的群 OpenID，逗号分隔；必须是 bot-scoped Group OpenID，不能填数字群号；管理员首次保存前的兜底列表 |

客户端密钥、加密密钥和 EVE 令牌不得放入 `VITE_*` 配置。命令、截图和文档中不要打印真实 `.env`。

## EVE 登录与本地入口

- 页面：[登录](http://127.0.0.1:5173/login)、[我的角色](http://127.0.0.1:5173/account)、[系统状态](http://127.0.0.1:5173/system)。
- EVE 开发者应用回调：`http://127.0.0.1:5173/api/v1/eve/callback`。使用 `127.0.0.1` 登录，不与 `localhost` 混用。
- 按[登录配置](integrations/eve-login.zh-CN.md)及[SeAT 默认 scopes](integrations/seat-login-scopes.zh-CN.md)配置现有应用，保留已有凭据和令牌密钥。
- 初次授予本站管理员：对已登录过的角色执行 `npm run access:admin -- --character ID`；撤销使用 `npm run access:admin -- --character ID --revoke`。这是明确的运维操作，不自动把第一个用户或游戏 Director 升为本站管理员。

## 迁移与构建边界

社区资料模块默认启用；旧的显式 `MODULES` 配置需追加 `community`。迁移 `00008` 建立资料与确认历史表，`00050`/`00051` 增加机器人事件与官方 QQ 一次性绑定码，`00052` 增加入群申请与群范围绑定，`00053`/`00054` 增加管理员群与 Bot 配置；旧用户首次访问角色页补填两项资料。需要官方 QQ 机器人时，可暂时在服务端环境配置 `QQ_BOT_APP_ID`、`QQ_BOT_APP_SECRET`、`QQ_BOT_API_BASE` 和可选的 `QQ_BOT_GROUP_OPENIDS` 作为首次迁移兜底，并把 `PUBLIC_ORIGIN/api/v1/community/qq/official/webhook` 配置到 QQ 开放平台；管理员随后在 `/account` 维护 AppID、API 地址、密钥和群配置，保存后数据库设置优先。App Secret 只在写入时从前端提交，后端使用由 `EVE_TOKEN_KEY` 派生的密钥加密，页面和接口不会回显。不要把 Bot Secret 放到日志或聊天中。

`dev` 和 `dev:external` 在 API 启动前执行迁移。`npm run start:api` 只编译和运行 API，不执行迁移，也不启动 Vite；单独启动前先运行 `npm run db:migrate`。`npm run build` 只生成构建产物，不执行部署或数据库迁移。

新增查询后执行 `npm run db:generate` 并核对生成结果。模块查询保存在各自的 `internal/store`，不手改生成文件。废弃的认证/考勤 SQL 草稿已清理；只使用 `migrations/` 与模块私有查询，不从历史文档恢复草稿执行。

当前正式迁移到 `00009`，foundation marker 为 1。迁移编号与模块兼容标记用途不同；API 不会在启动时自动修复缺失结构。更多约定见[模块开发](module-development.md)。

`00007` 自动将已有单角色用户的角色设为主角色；若非标准数据中已存在一名用户多个角色，则中止迁移，需先备份并制定明确回填方案，不可通过删除角色绕过。升级保留现有凭据与 `EVE_TOKEN_KEY`。操作入口及限制见[多角色说明](integrations/seat-multi-character.zh-CN.md)。

## 排查入口

| 现象 | 检查方式 |
| --- | --- |
| API 无法启动 | 检查配置名、数据库连接和模块依赖；先看当前终端的错误摘要，不输出密钥 |
| 就绪检查 503 | 检查开发库是否启动、迁移是否完成；`/health/live` 存活不代表数据库就绪 |
| 前端 API 访问失败 | 检查 API 监听地址、Vite 同源代理目标和服务是否在运行 |
| 登录提示未配置 | 检查后端凭据是否成对设置，access 启用时是否设置加密密钥，然后重启 |
| 授权失败或反复要求更新 | 按登录指南核对 origin、回调、应用 scopes 和玩家授权范围 |
| Go 数据库测试显示跳过 | 设置开发用 `TEST_DATABASE_URL` 后再运行对应测试，跳过不能写成集成测试通过 |
| 浏览器测试无法启动 | 安装 Playwright Chromium 或设置已安装 Chrome 的路径，先执行数据库迁移 |

配置错误、缺失 scope 与真实 EVE 服务故障需要分别定位，不通过关闭校验或直接修改身份表绕过。安全地保留请求 ID 以关联日志。

功能变更后的文档更新范围见[贡献与文档维护](../CONTRIBUTING.md)。

## 权限管理升级

执行 `npm run db:migrate` 至 `00009_access_management.sql` 后重启开发服务。页面 `/access` 仅对管理员或具备 `access.manage` 的账号开放。首位管理员先完成 EVE 登录，再执行 `npm run access:admin -- --character 角色ID`；不自动提权首个登录用户。管理 PUT 角色请求新增必填 `version`，DELETE 角色请求新增 `version` 查询参数，旧调用方须同步更新；详见[权限说明](integrations/seat-authorization.zh-CN.md)。

## ESI 同步升级（2026-09-14）

停止旧 API 后备份数据库及令牌密钥，执行 `npm run db:migrate` 应用 Goose 10 和 River 自有迁移，再启动 API。勿同时运行旧轮询器和 River。`/account` 提供本人角色状态，`/sync` 要求 `eve.sync.manage`；队列未就绪不会接受刷新。配置、保留策略与恢复见[中英文运行指南](integrations/esi-sync.zh-CN.md)。

## 合同同步升级

停止旧 API 并备份数据库后，`npm run db:migrate` 应用 Goose 15，再启动匹配的新 worker。首次合同目标立即到期，沿用共享限流、缓存与加密密钥；角色已授权清单缺少合同 scopes 时更新 EVE 授权。新增队列并发为 1，总数据 worker 并发为 3，资源参数仍需生产压测。见[合同运行说明](integrations/contracts.zh-CN.md)。

## SDE 自动更新

`SDE_AUTO_UPDATE=true`、`SDE_CHECK_INTERVAL=6h`、`SDE_WORK_DIR=.local/sde` 为默认配置。应用启用 eve 模块后，公共 SDE 任务在独立 `eve_sde` 队列运行，即使尚未配置 SSO。迁移 15 后须启动匹配的新 worker；旧版本不要消费新增的 `eve.sde-update.v1`。升级前停止旧 API 并备份，执行 `npm run db:migrate` 后重启。自动检查不需要添加 Codex 自动化或系统 cron；配置范围、失败恢复和版本固定见[SDE 中英文指南](integrations/sde-names.zh-CN.md)。

## ESI 客户端升级（2026-09-15）

当前 Goose 最新为 `00020_esi_sliding_budget.sql`。停止旧 API/worker、备份数据库与加密配置，执行 `npm run db:migrate` 后启动匹配代码；保留旧预算债务，不手动清空限流表。官方目录在根目录用 `node scripts/update-esi-catalog.mjs` 更新，固定客户端兼容日期，审查生成差异后测试构建。升级与回退详见[中英文客户端指南](integrations/esi-client.zh-CN.md)。

## 考勤模块升级

当前考勤/PAP 为 Goose 24，独立果壳币 exchange 模块使用 Goose 26，需在 MODULES 启用 exchange；手动/自动兑换和配置步骤见[兑换指南](integrations/exchange.zh-CN.md)，星系名称需 Goose 23 与 SDE mapper 2。迁移后运行 `npm run sde:import -- --latest` 导入名称，参见 [SDE 指南](integrations/sde-names.zh-CN.md)。停止本地 API 并备份开发库后运行 `npm run db:migrate`，保留原密钥及模块配置，再运行 `npm run dev:external`。新点名才生成舰船快照，旧装配不回填；成员详情“舰船与损失”提供状态与损失重试。新增任务及回退见[考勤指南](integrations/attendance.zh-CN.md)。

现有 `.env` 不会随默认值自动改写；启用考勤需在原 `MODULES` 追加 `attendance`，保留 identity/eve/access 依赖，运行 `npm run db:migrate` 后重启同版前后端。本期 Goose 21 不执行旧草稿 SQL。无 EVE 配置仍可读取已有考勤和统计，但不能抓取舰队/采集新在线状态。回退旧二进制前必须阻塞 online 目标，见[指南](integrations/attendance.zh-CN.md)。

## 舰船配置本地开发（Goose 27）

在既有 MODULES 中加入 fittings；先备份，运行 `npm run db:migrate`、`npm --prefix web ci`，重启本地服务后访问 `/fittings`。前端回归 `npm --prefix web run test`，构建 `npm --prefix web run build`；数据库验证设置仅指向隔离测试环境的 TEST_DATABASE_URL 后运行 `go test ./internal/modules/fittings ./internal/modules/eve ./internal/app`。npm ci 会安装锁定的 WASM/SDE 包。升级/禁用/旧二进制回退见[配装中英文指南](integrations/fittings.zh-CN.md)。

## 本地技能管理

保留 MODULES 原值并追加 `skills`，备份后运行 `npm run db:migrate` 和 `npm run build`，重启本地 API；前端 `/skills` 自动按模块声明显示。新迁移为 Goose 28，不改生产环境。官方目录重建和功能边界见[技能指南](integrations/skills.zh-CN.md)。

### 配装库本地升级

备份后运行 `npm run db:migrate` 应用 Goose 29 和 River；fittings 保留在 MODULES，技能生成入口需 skills。启用 fittings 会在基线 scope 上追加 esi-fittings.write_fittings.v1；开发者应用需启用，旧角色重新授权后才可保存到游戏。当前页面不再提供属性模拟，见[运行指南](integrations/fittings.zh-CN.md)。
## 账号合并本地升级（首次 Goose 30）

“我的角色”增加合并账号流程。先停止 API/worker 并备份数据库与 `.env`，执行 `npm run db:migrate` 维护 Goose 30 和 River，再启动同版前后端。没有新增 scopes 或 MODULES；真实账号不自动合并。已产生合并记录时 Down 会拒绝删除原归属追溯；使用兼容 schema 回退。使用与验证范围见[账号合并指南](integrations/account-merge.zh-CN.md)。

## 全站布局与浏览器回归

本机安装 Chrome 和 Edge、启动本地 API 与 5173 前端后，在仓库根目录运行：

```powershell
npm --prefix web run build
npm --prefix web run test:e2e -- --config playwright.layout.config.ts --workers=3
```

`playwright.layout.config.ts` 使用 Chrome/Edge 独立测试会话，各有桌面与 Pixel 7 模拟视口。页面状态检查另覆盖中英文、1440/375/320px；不等同于真实 Android/iOS 设备。若复用的是 Vite preview，须先构建前端。业务写入测试由接口夹具拦截，不使用用户的浏览器登录状态。

结果默认写到 `.local/layout-regression/results.json`，截图与失败 trace 在其 `results/` 子目录。可用 `LAYOUT_RESULT_DIR`、`LAYOUT_REPORT_FILE` 指定复测输出位置，以保留首次证据。状态与权限夹具只证明前端行为，不代替生产权限验收或 ESI 真实联调。

该配置排除 `interaction-diagnostics.spec.ts`：它验证仅在开发构建启用的诊断 worker，应另用 Vite dev 与默认测试配置运行。发布构建仍运行菜单切换和弹窗关闭后的可操作性测试；不据此宣称已修复用户原 Edge 配置中的卡顿。覆盖范围见[排版检查记录](ui/layout-review-2026-09-20.md)。

数据库慢查询观测：DB_SLOW_QUERY_MS 默认 500 毫秒（0 关闭），仅记录安全标识与耗时。Goose 46 并发建立历史保留期索引，详细边界与复测方法见[数据库性能](database-performance.md)。

## 本地军团介绍首页（2026-09-23）

原开发服务启动后访问 `http://127.0.0.1:5173/` 查看公开介绍；成员工作台位于 `/workspace`，独立登录页仍是 `/login`。前端 `npm ci` 安装锁定的 GSAP 依赖，无新增环境变量、scope 或数据库迁移。首页只依赖会话查询决定入口，后端不可用时介绍仍可阅读。见 [页面说明](ui/homepage.md)。
