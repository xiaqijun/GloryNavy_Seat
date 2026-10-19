# 公开首页层级优化（2026-10-20）

## 范围

用户要求使用已安装的 UI UX Pro Max 优化未登录首页。本轮保留已确认的五屏翻页方式和视觉基准，调整介绍、活动、保障与招募的分组及主次。页面参数见[首页](../ui/homepage.md)。没有后端、权限、业务数据、依赖或新图片变更。

技能搜索采用 `typography hierarchy --domain ux` 的字号层级建议及 `effect cleanup event listener --stack react` 的清理约束；最初 `progressive disclosure content hierarchy` 的首条为面包屑，与单页无关，未采用。未用技能替换已确认配色、图标库或全站主题，未改本机技能。

## 影响与验证

- GitNexus upstream：FleetChapters 仅由 LandingPage 调用，LOW；LandingPage 未解析出直接调用者，已人工核对 system 模块在 `/` 动态加载。索引初查落后 35 次提交，已完成刷新并复查，结果一致；不能将零调用视为无影响。刷新工具自动改写的 AGENTS/CLAUDE 文件已恢复原文，不随本轮交付规则变化。
- TypeScript 与 Vite 构建通过；既有大 chunk 提示保留，无新增依赖。
- 受影响源码 lint 无告警；全前端 lint 退出 0，13 条既有告警位于 QQ、福利、贷款和预警模块，与本轮无关。5 项 i18n 单测通过。
- 原首页 20 项回归通过：公开 API 白名单、匿名门禁、登录 POST、会话故障、未知/过期数据、双语、导航、触摸、键盘、复制及离页恢复。
- 1440/768/375/320px × 中英文共 8 项逐屏布局检查通过，检查页面宽度、锚点避让、桌面一屏高度及折叠要求展开后末项可达。目视复核后修正窄屏英文统计标签两行高度，使读数基线一致；最终构建复跑 8 项均通过。
- 最终截图：[桌面活动](../ui/reviews/public-home-ux-20261020/1440-zh-CN-fleet.png)、[介绍与统计](../ui/reviews/public-home-ux-20261020/1440-zh-CN-strength.png)、[手机招募](../ui/reviews/public-home-ux-20261020/375-zh-CN-join.png)、[窄屏英文统计](../ui/reviews/public-home-ux-20261020/320-en-strength.png)。其中统计为测试夹具，不是生产军团数据。
- GitNexus 全量差异与 compare/main 检查为 MEDIUM，4 条首页语言/样式流程，范围符合预期；最终暂存检查另在提交前执行。
- 本轮线上浏览器读取超时；后续本地截图只用于本地视觉验证，不能冒充生产客户端验收。

## 发布

尚未发布；生产仍为 `v0.1.0-public-home-pages-20261019`。计划只切换静态前端，保留上一版目录，不改后端或数据库。
