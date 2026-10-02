# Third-party source notices

## EVE 界面术语（2026-09-20）

`web/src/lib/eve-terminology.json` 的钱包与军团职务名称来自 CCP 官方 Tranquility SDE build 3503375 的 accountingEntryTypes / corporationRoles，旧项目船型来自 types；合同/槽位等短标签来自合法安装的同版本 Tranquility 客户端本地化资源。名称版权归 CCP，沿用 EVE 开发者许可及相关知识产权要求，不声明为项目原创。只提交所用短标签及来源/hash，不分发完整客户端、pickle、源码或游戏执行文件；提取器不执行游戏代码。来源键、复现命令及应用文案边界见[术语指南](integrations/eve-terminology.zh-CN.md)。

`web/src/components/ui/button.tsx` and `card.tsx` are adapted from [shadcn/ui](https://github.com/shadcn-ui/ui), retrieved through the shadcn CLI. Project adaptations include Corporate Clean styling, touch target sizing and local imports. Dependency packages retain their own licenses.

## Login visual assets

The login page uses the Glory Navy corporation logo from CCP's image service and the official EVE SSO button served by CCP. Their use does not make them part of the shadcn/ui MIT license. Source URLs, corporation identification and the separate AI-generated cruiser artwork are recorded in [the asset notes](../web/public/images/README.md). The generated artwork is decorative, not an official EVE screenshot or ship identification.

Official login button source: [CCP EVE SSO button](https://web.ccpgamescdn.com/eveonlineassets/developers/eve-sso-login-black-large.png).

The account page loads character portraits and corporation logos for the IDs returned by the application's APIs from [the EVE image service](https://developers.eveonline.com/docs/services/image-server/). These are CCP-hosted images; the app uses Lucide placeholders if an image is unavailable.

## shadcn/ui — MIT License

Copyright (c) 2023 shadcn

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

## 合同物品图标

合同查看页通过 EVE Image Server 的 types/{type_id}/icon 读取物品图标；属于 CCP/EVE Online 资源，不转为本项目原创资产。沿用已有 EVE 图片资源声明，名称由公开 ESI 名称接口补充；失败时使用 Lucide 后备图标。

## EVE 官方静态数据

物品中英文名称来自 CCP 的 [Tranquility 官方 SDE](https://developers.eveonline.com/docs/services/static-data/)。本地仅导入 type-names 用于合同物品展示，不把官方数据声明为本项目原创。ZIP 存放在忽略的 `.local/sde/`，来源、构建及校验记录见[SDE 运行说明](integrations/sde-names.zh-CN.md)和[核验记录](integrations/eve-sources.md)。

## Apache ECharts / ZRender

令牌桶接口消耗比较和考勤每日在线图表使用 Apache ECharts 6.1.0（Apache-2.0）及其 ZRender 渲染依赖（BSD-3-Clause），按需加载柱状图、坐标系和 SVG 渲染器。主题、中文标签与图标来自项目自己的组件约束。

上游：[Apache ECharts](https://github.com/apache/echarts)、[ZRender](https://github.com/ecomfe/zrender)。发布产物保留 [ECharts LICENSE](../web/public/third-party/echarts/LICENSE)、[NOTICE](../web/public/third-party/echarts/NOTICE) 和 [ZRender LICENSE](../web/public/third-party/zrender/LICENSE)。

## EVEShipFit 模拟引擎与配装数据

2026-09-15 配装页新增的舰船 render 图片由 CCP Image Service（`images.evetech.net/types/{type_id}/render`）提供；装备图标仍复用已有 EveImage。`web/src/modules/fittings/group-names.json` 来自官方 SDE 3503375 的 `groups.jsonl`，归属沿用 CCP 原始数据声明，可用 `scripts/fitting-group-names.py` 重建。页面布局参考游戏、Pyfa 与 EVE Ship Fit，组件为本项目实现，没有复制上述工具的界面代码或分发其截图；[调研记录](ui/fittings-research-2026-09-15.md)保留参考链接。

舰船模拟采用 [Dogma engine](https://github.com/EVEShipFit/dogma-engine) 10.3.0（MIT）与 [SDE patched](https://github.com/EVEShipFit/sde-patched) 2.3503375.0（上游补丁 MIT、原始 EVE 数据归 CCP）。WASM/SDE 随本站静态资产分发，保留 [引擎 LICENSE](../web/public/third-party/eveshipfit/dogma-engine/LICENSE)、[补丁 LICENSE](../web/public/third-party/eveshipfit/sde/LICENSE) 和 [CCP LICENSE.EVE](../web/public/third-party/eveshipfit/sde/LICENSE.EVE)。来源版本与更新规则见[接入说明](integrations/fittings.zh-CN.md)。不将 CCP 的数据、舰船或装备图标声明为本站原创；非 CCP 官方模拟器。

## 技能分类与目录

`internal/modules/skills/catalog.json` 来自 CCP 官方 EVE SDE build 3503375 的已发布 category 16 技能与组中文名，生成脚本 `scripts/skill-catalog.py`。作为名称后备、分类及类型校验；数据继续受 [EVE Developer License Agreement](https://developers.eveonline.com/license-agreement) 和 CCP 相关知识产权约束。不是本项目原创游戏数据。

## 军团配装参考（2026-09-15）

`scripts/fitting-reference.py` 从 CCP 官方 SDE build 3503375 生成 `internal/modules/fittings/reference.json`（类型名称、槽位与技能前置）。EVE 数据及图片归 CCP；不改变原数据归属。当前配装页面已停止加载 Dogma/WASM 模拟资源，旧源码/依赖的声明仍保留。

## Public homepage artwork and animation (2026-09-23)

The real Avatar ship image is from the official historical NetEase EVE ship gallery; original ownership remains with CCP / the original rights holders, and the footer identifies the site as a player corporation website. The independent warm starfield is AI-generated decoration. Source URLs and dimensions: [asset notes](../web/public/images/README.md). Existing corporation logo remains unchanged.

Homepage animation adds GSAP 3.15.0 and @gsap/react 2.1.2. Preserve their distributed license files under the installed packages; version selection is locked in web/package-lock.json. The application does not redistribute them as a standalone animation authoring product.
