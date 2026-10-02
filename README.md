# GloryNavy

面向 EVE Online 国际服 Tranquility 的自研军团管理平台。Go + React + PostgreSQL，采用 Corporate Clean 中文界面与显式模块注册架构。

已上线公开军团首页、EVE 登录与多角色/账号合并、权限与成员管理、ESI/SDE 同步、合同与钱包、考勤与军团/联盟 PAP、配装与技能、福利、奖励兑换、审批中心和工作台。支持中英文。真实游戏验收、联盟 PAP 自动兑换及社区机器人等剩余工作见[待办清单](docs/backlog.md)，当前生产版本见[项目状态](docs/project-status.md)。

## 快速启动

安装仓库所需的 Go、Node.js 与 Docker Compose 后，在项目根目录执行：

```sh
npm run setup
npm run dev
```

打开 [登录页](http://127.0.0.1:5173/login)。尚未配置 EVE 应用时会显示必要提示；开发与登录配置见[开发指南](docs/development.md)。已有独立 PostgreSQL 的环境可改用 `npm run dev:external`。

脚本会在缺失时创建本机 `.env`，开发启动会先迁移数据库。不要覆盖已有凭据或令牌加密密钥。具体依赖、配置、测试、停止方式和本机数据库说明集中维护在开发指南。

## 项目文档

| 入口 | 内容 |
| --- | --- |
| [文档目录](docs/README.md) | 全部文档及职责划分 |
| [项目状态](docs/project-status.md) / [待办清单](docs/backlog.md) | 当前交付与验证边界 / 未完成事项及验收条件 |
| [开发指南](docs/development.md) | 运行、配置、迁移、检查与排查 |
| [技术栈](docs/tech-stack.md) / [架构](docs/architecture.md) | 已采用组件、设计基线和扩展边界 |
| [模块开发](docs/module-development.md) | 增加后端与前端模块 |
| [UI 规范](docs/ui-design-rules.md) / [登录页](docs/ui/login.md) | 全局基准与页面适配 |
| [中英文接入文档](docs/integrations/README.md) | EVE SSO、ESI、SDE 与 SeAT 授权 |
| [API 契约](api/openapi.yaml) | 路径、请求响应和鉴权 |
| [变更记录](CHANGELOG.md) / [贡献约定](CONTRIBUTING.md) | 交付变化与同步维护规则 |

## 目录

```text
cmd/                    API、迁移和管理员命令入口
internal/app/           宿主组装与模块启用
internal/module/        模块契约、依赖与权限声明
internal/modules/       业务模块及各自私有 store
migrations/             Goose 发布迁移
api/                    OpenAPI 契约
web/                    React 页面、共享组件与静态资源
scripts/                开发、生成与检查命令
docs/                   当前规范、接入指南、历史记录及草稿
```

功能变更必须在同一次交付中同步相关文档，规则见 [AGENTS.md](AGENTS.md) 与 [CONTRIBUTING.md](CONTRIBUTING.md)。
