# 公开首页分屏翻页（2026-10-19）

## 范围

用户明确要求类似采星者的翻页式首页，并减少文字堆砌。仅改公开 `/` 的前端内容组织和交互，详细设计见[首页](../ui/homepage.md)。后端、公开接口、登录流程、权限和业务数据不变。

五屏为首屏、军团介绍、军团活动、成员保障、加入我们。沿革和公开数据合并，活动/保障各三项短句，入团要求默认折叠。原生滚动吸附、动态页头避让、页码及菜单高亮；内容可超出一屏，不裁切。取消本页 GSAP，离开后移除滚动规则。

## 影响与验证

- GitNexus upstream：PublicStrength、FleetChapters、JoiningSteps 各 1 个直接调用方 LandingPage，LOW；涉及首页的语言/样式路径。LandingPage 动态入口未识别，人工确认 system 模块独立 `/` 加载。索引落后 33 次提交，结果已结合当前源文件补查，不能将零调用当成无影响。
- 参考页正文可读取，浏览器两次超时，未声称完成采星者的视觉复刻；按用户明确的翻页交互实现，本站浏览器验证另记。
- TypeScript、Vite 生产构建、受影响源码与用例 lint 通过。保留现有大 chunk 提示，无新增依赖。
- 原首页 17 项回归全部通过，包括公开 API 白名单、会话故障、登录 POST、工作台门禁、数据未知/过期、中英文和导航。
- 最终构建 8 项分屏布局通过：1440/768/375/320px × 中文/英文，每项逐屏检查定位、宽度、默认折叠及展开后末项可读，并保存截图。首轮开发服务器有 1 次懒加载超时，最终构建复验通过。
- 最终构建 8 项行为场景均已通过：语言切换、4 宽度导航、滚轮/键盘/离页恢复、旧 `#about`/短屏/复制、触摸翻页。滚轮场景首次在反向吸附尚未结束时点击下一页失败；用例改为等待实际吸附到位并点击确定目标后连续 3 次通过。没有因此增加滚轮拦截或固定等待。
- 手机英文菜单缩短为 About us / Activities / Support / Join us，招募删除重复说明；128px 军团图标按原始尺寸显示。已复核[桌面首屏](../ui/reviews/public-home-pages-20261019/1440-zh-CN-home.png)、[介绍](../ui/reviews/public-home-pages-20261019/1440-zh-CN-strength.png)、[活动](../ui/reviews/public-home-pages-20261019/1440-zh-CN-fleet.png)、[375px 招募](../ui/reviews/public-home-pages-20261019/375-zh-CN-join.png)、[320px 英文](../ui/reviews/public-home-pages-20261019/320-en-strength.png)。截图中的统计为测试夹具，不是生产军团数据。
- GitNexus 提交前 staged 与 compare/main 检测均 MEDIUM：16 个文件、14 个变更条目、4 条首页语言/样式流程，范围符合预期；`git diff --cached --check` 通过。

## 发布（北京时间 2026-10-19 23:59:36）

- 前端 `v0.1.0-public-home-pages-20261019`，来源干净且推送的 main `363e78795039f6d8cb85f5248a63ed1917127c08`，构建元数据 UTC `2026-10-19T15:58:39Z`。145 个构建文件与测试冻结清单逐一匹配，发布包和线上完整 SHA-256 检查通过。
- 通过现有 `activate-edge-release.sh` 切换前端；上一版 `public-home-nav` 目录保留。未发布后端、迁移、修改环境配置或写入业务数据。
- 线上 `/`、`/login`、`/workspace` 均 200；匿名 modules、eve/status、approval/context、wallet/summary 均 401。HTML、主 JS、首页 JS/CSS 与发布产物字节一致，OpenResty 配置检查通过。
- HTML SHA-256：`266ff8cc2a9c5b2d9016c9923dcad70b630f1bc5781178c546c1dde77f668b08`。主包 `index-D6MiBNnv.js`、首页 `landing-page-DFyw0htP.js` / `landing-page-D01CAfJ2.css`。
- 最新页面本地视觉与交互矩阵通过；生产完整浏览器目视矩阵仍在 WEB-01，未将资源校验冒充实际客户端验收。忽略目录 `.local/home-pages/` 保留构建、专项日志、原始截图和发布辅助脚本。
