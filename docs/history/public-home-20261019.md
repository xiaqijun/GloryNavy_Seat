# 公开首页优化 · 2026-10-19

范围为未登录时可访问的 `/`；沿用公开页面的会话按钮，工作台不变。无后端变更、迁移、权限或新数据接口。页面细节见[首页记录](../ui/homepage.md)。

## 实现

军团数据前移，常驻导航可直接到各段；合并重复内容和招募区，保留船体展示与缩短后的 GSAP 转场。击毁价值采用英文紧凑单位，异常与未知仍明确显示。素材来源沿用原记录。

## 验证

- TypeScript、Vite 构建及受影响文件 lint 通过。
- 最终 17 项首页浏览器回归通过：匿名 API 边界、会话异常、POST 登录、语言切换、未知与过期公开数据、320/375/768/1440px 中英文、锚点避让、减少动态效果及正常转场正反滚动、离页清理。
- 8 组中英文布局截图及一次交互复核通过；验证跳转主要内容的键盘操作、复制群号成功及失败反馈、手机登录错误排版。
- 首轮旧用例仍期待保留登录 error 参数、招募合并后标题定位出现歧义；已按现有错误提示消费机制与唯一标题修正。页尾锚点验收允许文档到底时停止自然滚动，不额外增加留白。
- 视觉复核修正手机错误提示撑高固定页头，并去掉复制成功后的重复可见文字。最终回归针对最终构建执行。
- GitNexus 组件影响为 LOW；LandingPage 动态入口未解析到调用者，已核对 system.landing 的模块加载。索引落后 25 次提交，尝试刷新未完成，补充源码与路由检查。提交前暂存和 main 比较均为 MEDIUM，14 文件、12 个符号、4 条首页流程，范围符合预期。
- 减少动态效果、相同模拟数据下，1440px 全页从 4318px 到 2734px，375px 从 5198px 到 3382px；普通桌面仍保留固定舰队章节。

截图：[桌面](../ui/reviews/public-home-20261019/1440.png)、[手机](../ui/reviews/public-home-20261019/375.png)、[320px 英文](../ui/reviews/public-home-20261019/320-en.png)、[登录异常](../ui/reviews/public-home-20261019/login-error.png)、[招募](../ui/reviews/public-home-20261019/join.png)。截图为模拟公开 API，不代表实际军团读数；未执行真实 EVE 授权或加入社群。

## 发布

- 前端 `v0.1.0-public-home-optimize-20261019`，来源为干净且推送的 main `d76d31a03b50151a8963c7cd2fb8ac9dd8862fea`；143 个打包文件与最终浏览器测试使用的构建逐一匹配。
- 完整 SHA256SUMS、公开 `/`、`/login`、`/workspace` 均通过；匿名 modules、eve/status、approval/context、wallet/summary 均为 401。OpenResty 配置检查通过。
- 线上 HTML SHA-256：`4778bdf407bddd376de7ef3dd25183d2ac5cce9d769143e00729917af2feac6f`。
- 主脚本 `index-CgO5ulvz.js`：`facd7c98ae8edf468b552be30739f5d3ff069c3c7e8d64da5eaf7ec8f5c7f03e`。
- 首页脚本 `landing-page-CabTNc7r.js`：`e7535cba4f6589e3497d045f7e577f74eeb2783f9ac9f79d52e3ccd28fb46bab`。
- 首页样式 `landing-page-CbgNJuzp.css`：`7cba671e0802a62e52b7c2ba475b5e9f51e8ee36b5f0c75ef9cebbd37172669b`。
- 仅前端发布，后端仍为 `v0.1.0-exchange-withdraw-20261019`、Goose 103；未重启后端或操作业务数据、社群消息。
- 前端可切回上一版 `v0.1.0-exchange-debt-note-20261019`，不涉及数据库回退。
