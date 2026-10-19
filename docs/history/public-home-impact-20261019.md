# 公开首页视觉强化 · 2026-10-19

用户在首轮精简后反馈“整体太普通，缺少视觉冲击”。本轮聚焦公开 `/` 的首屏比例与整体构图，工作台、业务数据和登录流程不变。前一轮证据保留在[首轮记录](public-home-20261019.md)。

## 实现

扩大品牌标题与官方神使级舰体，以暖金色星云和细线轨道构成首屏，军团实力接入同一深色画面。浅色中段将三项舰队生活并排展示，移除第二张重复舰船和固定滚动切换；手机自然单列。没有新图片、依赖、接口、迁移或权限。具体尺寸见[首页说明](../ui/homepage.md)。

## 验证

- TypeScript、Vite 构建与受影响文件 lint 通过。
- 最终构建的 17 项首页回归通过：匿名接口白名单、未知与在线数据过期、会话异常、EVE POST 登录、320/375/768/1440px 中英文、锚点避让、减少动态效果、正常动效的正反导航与离页返回。
- 8 组布局截图及 1 项招募/键盘交互检查通过，复制成功/失败和手机登录错误均可读；另导出 1440×1000 首屏截图。检查后修正平板舰体与正文的重叠，最终宽度 900px 以下使用上下构图。
- In-app browser 实际打开本地页面，检查首屏及舰队生活导航。没有执行真实 EVE 授权或加入社群。
- 最终桌面全页约 2175px，375px 约 2987px；移除原固定章节的额外滚动长度。

截图：[首屏](../ui/reviews/public-home-impact-20261019/hero.png)、[桌面](../ui/reviews/public-home-impact-20261019/1440.png)、[手机](../ui/reviews/public-home-impact-20261019/375.png)、[768px 英文](../ui/reviews/public-home-impact-20261019/768-en.png)、[320px 英文](../ui/reviews/public-home-impact-20261019/320-en.png)。测试截图使用模拟公开 API，不能作为生产军团读数。

GitNexus 在编辑前分析 PublicStrength、FleetChapters，均为 LOW，唯一直接调用为 LandingPage，涉及首页语言/样式流程。LandingPage 动态入口为 UNKNOWN，已核对 system.landing 的本地动态加载。索引落后 27 次提交，采用源代码与专项回归补足，不把未发现调用解释为没有影响。

## 发布

- 前端 `v0.1.0-public-home-impact-20261019`，干净且推送的 main `9869df5fae80afaacb21476c5dfe07e56e876bfd`，143 个文件与最终测试构建逐一匹配。
- 提交前 GitNexus 暂存与 main 比较均为 MEDIUM：14 个文件、10 个符号、4 条 LandingPage 流程，范围符合预期。
- 首次执行上传的暂存脚本因 CRLF 在第一行退出，未切换生产；转换 LF 后暂存、清单校验与激活成功。
- 全量 SHA256SUMS、公开 `/`、`/login`、`/workspace` 均通过；匿名 modules、eve/status、approval/context、wallet/summary 均为 401。OpenResty 配置正常。
- 线上 HTML SHA-256：`2c019506568c013f25f74aa6486c5e25d7b60b1b363bc3959f916ac1c1a9736f`。
- 主脚本 `index-BqbeqoDC.js`：`b481f3902b98b539d129fd27caee27293fbe808eaca721f850548b6eeb7ac10f`。
- 首页脚本 `landing-page-BjcEV2eg.js`：`54ae93030515f7384882188057a6cdd441a4a2ecec2ba1bded0f3edfc3a976c4`。
- 首页样式 `landing-page-C20eZorI.css`：`49c0f90dc7b35cc0210a2ab8cea66b26d62920b8cdfa77e28aae8caeabea2cfc`。
- In-app browser 已在生产打开新版页面并读取真实公开统计，首屏截图见[线上实拍](../ui/reviews/public-home-impact-20261019/live.png)。未代用户登录或发送社群消息。
- 仅前端发布，后端保持 `v0.1.0-exchange-withdraw-20261019`、Goose 103；无后端重启或业务写入。可切回前端 `v0.1.0-public-home-optimize-20261019`，不涉及数据库回退。

## 撤回

2026-10-19 用户反馈“更丑了”，已通过既有边缘发布脚本切回 `v0.1.0-public-home-optimize-20261019`。来源与完整清单校验通过，OpenResty 配置正常，首页 200。恢复首页三份源码及其回归用例，不回退其他业务改动，不修改数据库。二轮截图留作否决方案的历史证据；后续先展示构图预览，选择后再实现。

恢复后再次验证：公开 `/`、`/login`、`/workspace` 为 200，匿名业务接口为 401，HTML 及主包、首页 JS/CSS 与旧版文件逐字节一致。源码与用例逐字节等于 `d76d31a`，没有重跑历史测试或将历史测试记作本轮通过。
