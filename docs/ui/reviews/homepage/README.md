# 首页泰坦素材参考

2026-09-23 下载并目视核对，仅用于首页设计调研，未部署为网站资源。

## 网易舰船展示原图（优先候选）

用户提供网易舰船展示链接；已核对页面标题、该站公开舰船目录和对应 PNG，并检查透明通道。目录由该站 `index-liuma_061c3d2.js` 引用的公开 `sixhorse.game.163.com/news/outer/newslist.do` 返回，`contentkind=33139`。

- `eve-revelation-netease-reference.png`：用户原链接指向**神示级 Revelation（无畏舰）**，并非泰坦；1400×980，索引色 PNG，含透明信息，转换读取 alpha 范围 0–255，文件本身未重编码。
  - [页面](https://evepc.163.com/jczt/amdg/20200709/33139_891738.html)
  - [原图](https://nie.res.netease.com/r/pic/20200716/52f1b42b-dc16-4f1f-b385-bf500e74e9f2.png)
- `eve-avatar-netease-reference.png`：在同一官方目录找到的**神使级 Avatar（泰坦）**；1400×980，索引色 PNG，含透明信息，alpha 范围 0–255，原始字节保留。
  - [页面](https://evepc.163.com/jczt/amdg/20200709/33139_891741.html)
  - [原图](https://nie.res.netease.com/r/pic/20200716/e704099c-21e8-4861-8de8-c0fe4273829b.png)

这类透明底真实舰船图可以与独立宇宙背景分层，实现船体与背景的小幅相对位移，不需生成模型重画舰体。1400px 为整张画布宽度，船体并未占满，不作为任意超大放大的清晰度保证。素材来自国服官方历史展示站，仅作视觉来源，不改变本项目国际服接入与数据规则。

## 历史官方宣传图

- `eve-capital-balance-reference.jpg`：3840×2160；神使级舰队宣传图，含版本文字，未编辑。
  - 来源：[Surgical Strike – Coming 15 April](https://www.eveonline.com/news/view/surgical-strike-coming-15-april)，2020-04-07。
  - [原图](https://images.ctfassets.net/7lhcm73ukv5p/30p2HqChODTvT7ZtdBhL6y/ca05b8fa09e861ae9b09cc58b96bd7aa/Capital_Balance_3840x2160_2.jpg)。
- `eve-avatar-reference.jpg`：1024×512；四艘神使级 SKIN 宣传对照，未编辑。
  - 来源：[Surgical Strike is Live!](https://www.eveonline.com/news/view/surgical-strike-is-live)，2020-04-15。
  - [原图](https://images.ctfassets.net/7lhcm73ukv5p/1E6cpAzP31jc390NHFo1YS/988a6863d5d5451536fb2d3f38c57cc5/Avatar.jpg)。

这两张为 EVE 官方历史发布素材，不是 GloryNavy 舰队或资产的证明。下载用于参考不代表已经确认正式网页复用授权，实际选材时再核对适用条件与署名。不得移除署名、品牌水印或伪称自产。当前不改动 `web/public/images/` 的既有文件。

## 本地实现（2026-09-23）

用户批准方案 1 并要求统一页尾后，真实 Avatar PNG 已原样复制至 `web/public/images/home-avatar.png`；其他参考舰船未投入页面。新增独立暖金深空背景，首屏、中段近景、招募尾部共同使用，完整来源见 [公共素材](../../../../web/public/images/README.md)。已保存桌面及手机英文截图与 [Design QA](design-qa.md)，生产未变。
