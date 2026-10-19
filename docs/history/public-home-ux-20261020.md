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
- GitNexus 最终暂存与 compare/main 检查均为 MEDIUM：13 个文件、8 个变更条目、4 条首页语言/样式流程，范围符合预期；暂存差异检查和受影响文档链接检查通过。
- 本轮线上浏览器读取超时；后续本地截图只用于本地视觉验证，不能冒充生产客户端验收。

## 发布（北京时间 2026-10-20 00:32:46）

- 前端 `v0.1.0-public-home-ux-20261020`，来源干净且推送的 main `653c59b6680a777d618dd0716754f982577cfde1`；145 个构建文件与测试冻结清单逐一匹配，线上完整 SHA-256 清单通过。
- 通过既有静态发布脚本切换；上一版 `v0.1.0-public-home-pages-20261019` 目录保留。只更换前端，没有后端重启、迁移、配置或业务数据写入。
- 线上 `/`、`/login`、`/workspace` 均 200，匿名 modules、eve/status、approval/context、wallet/summary 均 401，OpenResty 配置检查通过。
- 公网 HTML、主 JS、首页 JS/CSS 与测试产物字节一致。HTML SHA-256 `fdb507ae5083281be247aae2eff149acf1346e35699752ad2a73b4229d94bead`；主包 `index-Pb1rymQe.js`、首页 `landing-page-CfMQDmtQ.js` / `landing-page-DQm6UYmf.css`。
- 本轮本地视觉矩阵通过，线上完整浏览器目视矩阵仍在 WEB-01，不把资源字节核对当作实际客户端验收。忽略目录 `.local/home-ux/` 保存构建清单、原始截图和发布辅助脚本。

## 移除在线角色

2026-10-20 用户追加要求：公开首页移除“在线角色”。只保留军团角色、本月击毁、击毁价值，桌面三列、600px 以下三行，手机读数右对齐。清理首页在线到期定时器和动态刷新周期，战绩维持 30 秒查询；后端接口和成员区在线采样不变。

- GitNexus upstream：PublicStrength 仅由 LandingPage 调用，LOW，涉及首页语言与样式流程。索引落后两次首页布局/文档提交；当前函数源码已直接核对。
- 更新原首页统计断言，移除不再适用的首页在线到期用例；未为纯排版添加实现镜像测试。
- TypeScript、Vite 构建、受影响代码 lint 通过；19 项首页回归全部通过，包含匿名权限、未知数据、语言与各尺寸布局、登录和滚动行为。
- 1440/375/320px × 中英文 6 项逐屏检查通过；已目视复核[桌面三项统计](../ui/reviews/public-home-stats-20261020/1440-zh-CN-strength.png)与[320px 英文三行](../ui/reviews/public-home-stats-20261020/320-en-strength.png)。截图为本地测试夹具，不是生产数据。
- 待发布，生产仍为 `v0.1.0-public-home-ux-20261020`。
