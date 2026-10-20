# 公开首页 SEO 与 GEO

2026-10-20 用户要求优化首页 SEO/GEO。当前已实现并完成本地验证，尚未发布；正式结果在本文追加。仅前端和边缘路由，后端及 Goose 103 保持不变。

## 问题与实现

原首页 HTTP 文档只有加载占位，正文依赖 JavaScript；robots.txt 和 sitemap.xml 都被 SPA 回退为首页 HTML。现在构建以同一 React App/首页组件输出完整公开正文，不按 User-Agent 区别内容、不复制单独给爬虫的隐藏页面。构建使用空查询缓存并禁止 fetch，中文匿名页面可直接 hydration，英文偏好与登录错误按原流程挂载。

首页统一管理标题、描述、canonical、Open Graph/Twitter 及 Organization/WebSite/WebPage JSON-LD。组织事实仅使用可见的军团名称、游戏环境、服务和公开战绩链接，不写角色名单、实时人数、PAP、钱包或固定福利数额；不给晨曦沿革与宁静游戏实体混写创始日期。

只生成一个首页 sitemap URL，旧域名继续永久跳转。robots 对首页、公开资源与发现文件开放，其余路径禁爬；单独的成员壳包含 noindex，真实边缘路由另加 X-Robots-Tag。JavaScript 从首页跳到成员页时删除公开 canonical、分享和 JSON-LD。实际后端鉴权不依赖 robots。

llms.txt 是与公开正文一致的简短导航摘要，属于可选阅读入口，不是收录保证。现有五屏、入团要求、品牌素材和福利规则保持，介绍口号换为明确的军团身份说明。

## 本轮验证

- TypeScript、受影响代码 oxlint、完整 Vite 构建及离线预渲染通过。保留已有大 chunk/构建插件耗时提示。
- 5 项中英文词表/切换测试通过。
- 26 项首页浏览器检查中 25 项通过，含 6 项新增检查：原始 HTML/JSON-LD、发现文件、禁用 JS 阅读、中文 hydration/英文元信息、成员原始文档与 SPA 跳转清理、OAuth 错误。其余通过项覆盖匿名数据边界、登录、语言、四尺寸布局、统计失败状态和翻页操作。
- 原有滚动定位用例失败：连续导航后的目标顶部为 123px，断言预期 80px。对未修改的 `v0.1.0-alliance-pap-bindings-20261020` 产物执行同一用例得到完全相同失败，未修改测试掩盖问题。
- 4 项额外排版检查通过：1440×1000、375×812、812×375，以及 375px 根字体 24px；减少动态效果下内容、招募要求和宽度边界通过。桌面与手机介绍截图已目视核对。
- GitNexus 索引已刷新。App/LandingPage impact 仍返回 UNKNOWN（React 动态入口缺少调用边），人工核对启动、模块注册和页面引用，并以首页/成员切换测试补充。提交前 staged 与 compare main 的 detect_changes 均为 medium，23 个文件、5 条受影响流程集中在首页、元信息和翻译；未发现业务服务变更，不把零调用边视为无风险。

## 上线与恢复要求

前端必须来自干净且已推送的 main，产物与测试清单逐文件一致。保存旧静态链接及主站配置；同步切换预渲染产物与 edge-https.conf，执行 nginx -t 后平滑 reload。失败恢复二者，不能只回退到缺少 app.html 的旧前端。只更新该主站配置，不改其他虚拟主机、证书或 API 代理。

线上需确认首页原文、发现文件 MIME、canonical/分享图、私有页面 noindex、安全头、旧域名重定向及匿名业务拒绝。实际索引、搜索排名、AI 引用和站长平台提交未验收；不得写成已被搜索引擎收录。

## 采用的官方资料

- [Google JavaScript SEO](https://developers.google.com/search/docs/crawling-indexing/javascript/javascript-seo-basics)：原始可读正文、规范链接、状态和元信息。
- [Google AI 搜索说明](https://developers.google.com/search/docs/appearance/ai-features)：保持重要信息为可读文本，结构化数据与可见内容一致；没有专用 AI 文本文件的强制要求，收录与展示不保证。
- [OpenAI 爬虫说明](https://developers.openai.com/api/docs/bots)：搜索爬虫遵循 robots；本轮公开路径策略对标准爬虫统一，不以伪造访问或隐藏内容绕过限制。
- [Schema.org Organization](https://schema.org/Organization)：组织实体与公开身份链接。
