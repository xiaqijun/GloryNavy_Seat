# 公开首页招募内容整理 · 2026-10-19

## 来源与范围

用户明确否决此前视觉方案，并提出内容空洞、舰队生活复杂。根据本轮提供的军团招募介绍补齐沿革、具体活动、新人支持及入团要求；用户确认福利以站内规则为准，不刊登旧固定补助、返还或全额补损承诺。英文从中文重译，纠正原稿两服英文名称颠倒，旧 DKP 更新为 PAP／果壳币。正文和响应式规则见[首页说明](../ui/homepage.md)。

沿用恢复后的首轮外观、官方舰船与真实公开数据；删除重复舰船与固定滚动章节，改为六项活动自然阅读。只改公开首页，不涉及工作台、业务接口、权限、数据库或登录 scope。

## 影响分析

编辑前 GitNexus：FleetChapters 为 LOW，唯一直接调用为 LandingPage，关联首页语言与样式流程；LandingPage 和 english 词表未解析到上游，另核对了 system.landing 动态入口及词表使用。索引落后 30 次提交，不能把缺少调用边视作无影响，使用源码核对和首页专项回归补足。

## 验证与发布

- TypeScript、Vite 构建、受影响文件 lint 通过；5 项 i18n 单测通过。
- 最终构建 17 项首页回归通过，包含匿名 API 白名单、登录及会话异常、数据过期与未知值、320/375/768/1440px 双语、锚点定位、正常动效正反导航和离页返回。
- 8 组双语布局截图与 1 项招募/键盘交互检查通过，复制成功/失败、登录错误可读。检查桌面、375px 中文、320px 英文的全页，无文字重叠或水平溢出；桌面约 2268px、375px 约 3449px。没有执行真实 EVE 登录或社群消息。
- 截图：[桌面](../ui/reviews/public-home-content-20261019/1440.png)、[手机](../ui/reviews/public-home-content-20261019/375.png)、[768px 英文](../ui/reviews/public-home-content-20261019/768-en.png)、[320px 英文](../ui/reviews/public-home-content-20261019/320-en.png)。截图使用模拟公开数据，仅用于排版验证。
- 已从干净且推送的 main `5d5a4a805e0930cbd3a6eb53bff418faa00ed261` 发布前端 `v0.1.0-public-home-content-20261019`，145 个文件与最终测试构建逐一匹配。
- 提交前 GitNexus 暂存与 main 比较均为 MEDIUM，14 个文件、15 个索引条目、4 条 LandingPage 流程。索引偏移使同文件未编辑符号被纳入，已逐段核对：PublicStrength 查询与 JoiningSteps 未改动。
- 完整 SHA256SUMS、线上 `/`、`/login`、`/workspace` 200，匿名 modules、eve/status、approval/context、wallet/summary 401，OpenResty 配置正常。
- 线上 HTML SHA-256 `2efadd50b6401e1e94f8d7b0ac57b877d8ae38b12066d87db54fd8c3f9988504`；主包 `index-DTi---2R.js`、首页 `landing-page-CkgZGR_3.js` 及 `landing-page-B_tFENdA.css` 与本地发布文件逐字节一致。
- 后端 `v0.1.0-exchange-withdraw-20261019`、Goose 103 不变，无后端重启或业务写入；可切回 `v0.1.0-public-home-optimize-20261019`。

## 追加：一菜单一章节

用户要求上方菜单配有对应动效、一菜单一节。四项锚点保持一一对应，新增滚动位置高亮、150ms 指示线过渡及局部平滑滚动；不增加固定场景或强制整屏高度。尊重减少动态效果，监听随离页清理。编辑前重新分析 LandingPage，动态入口仍 UNKNOWN，已核对模块注册；本次无后端变更。

导航版类型/构建/lint 通过；最终构建 17 项首页回归和 9 项布局/交互检查通过。回归新增四种宽度双语各节唯一选中断言、正反导航、直接滚动和较高视口页底选中；移动端指示线见[截图](../ui/reviews/public-home-content-20261019/nav-mobile.png)。前端已由干净且推送的 main `b78cfa16749866cbd7e53f9b09b7f360003a3dce` 发布 `v0.1.0-public-home-nav-20261019`，145 个文件与测试构建一致。

- 提交前 GitNexus 暂存与 main 比较为 MEDIUM，8 个文件、5 个索引条目、4 条首页流程，范围符合预期。
- 完整清单、OpenResty 配置、三条公开页面 200、四条匿名业务 API 401 均通过。
- 线上 HTML SHA-256：`0e15d81193588598934282492e5681f6bd700460c840ad66c7fec8093799f8e8`。
- 主包 `index-Bh5pXaIU.js`、首页 `landing-page-CZNjXWKP.js` 与 `landing-page-BOdy3OqF.css` 已逐字节核验。
- 本地浏览器回归与手机截图已验证高亮。生产浏览器刷新完成，但后续读取超时，未将其记为线上视觉确认；线上产物与权限验证已独立完成。
- 导航版仅前端，后端及 Goose 不变，可切回内容版 `v0.1.0-public-home-content-20261019`。

## 外部参考调研

用户随后要求找其他军团介绍页借鉴。已通过公开网页和浏览器查看 KarmaFleet、Brave Collective、Dreddit，来源与可借鉴点记录于[首页说明](../ui/homepage.md#后续外部参考2026-10-19-调研)。未复制外站图片、整段文案或福利政策，未因此再次更换当前上线构图。
