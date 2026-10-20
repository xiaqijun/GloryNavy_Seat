import { readFile, writeFile } from "node:fs/promises";
import { createServer } from "vite";

const server = await createServer({
  server: { middlewareMode: true, watch: null },
  appType: "custom",
  logLevel: "error",
});
const escape = (text) =>
  text
    .replaceAll("&", "&amp;")
    .replaceAll('"', "&quot;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
try {
  const { renderHome, pageHeadItems, publicSiteMetadata, publicSiteURL } =
    await server.ssrLoadModule("/tooling/prerender-home.tsx");
  const original = await readFile("dist/index.html", "utf8");
  if (
    !original.includes('<div id="root"></div>') ||
    !original.includes("<!--public-head-->")
  )
    throw new Error("Public HTML template markers are missing");
  // Preserve a noindex application shell for every non-public route.
  await writeFile("dist/app.html", original);
  const fetch = globalThis.fetch;
  let rendered;
  try {
    globalThis.fetch = () => {
      throw new Error("Prerendering must not fetch live data");
    };
    rendered = renderHome();
  } finally {
    globalThis.fetch = fetch;
  }
  // React's resource hints belong in the head, not among hydration children.
  const preloads = rendered.match(/<link\b[^>]*rel="preload"[^>]*\/>/g) ?? [];
  for (const preload of preloads) rendered = rendered.replace(preload, "");
  const head = pageHeadItems(true, "zh-CN")
    .map(({ tag, attrs, text }) => {
      const attributes = Object.entries(attrs)
        .map(([key, value]) => `${key}="${escape(value)}"`)
        .join(" ");
      return tag === "script"
        ? `<script data-site-meta ${attributes}>${text}</script>`
        : `<${tag} data-site-meta ${attributes}>`;
    })
    .join("\n");
  const html = original
    .replace(
      /<title>.*?<\/title>/s,
      `<title>${escape(publicSiteMetadata().title)}</title>`,
    )
    .replace(/<meta data-site-meta name="robots"[^>]*>/, "")
    .replace("<!--public-head-->", `${head}\n${preloads.join("\n")}`)
    .replace(/<!--boot-fallback-start-->[\s\S]*?<!--boot-fallback-end-->/, "")
    .replace(
      '<div id="root"></div>',
      `<div id="root" data-prerendered="zh-CN">${rendered}</div>`,
    );
  if (!html.includes("674743797") || !html.includes('id="requirements-title"'))
    throw new Error("Public content was not fully prerendered");
  await writeFile("dist/index.html", html);
  await writeFile(
    "dist/robots.txt",
    [
      "User-agent: *",
      "Disallow: /",
      "Allow: /$",
      "Allow: /?",
      "Allow: /assets/",
      "Allow: /images/",
      "Allow: /robots.txt$",
      "Allow: /sitemap.xml$",
      "Allow: /llms.txt$",
      "",
      `Sitemap: ${publicSiteURL}sitemap.xml`,
      "",
    ].join("\n"),
  );
  await writeFile(
    "dist/sitemap.xml",
    `<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><url><loc>${publicSiteURL}</loc></url></urlset>\n`,
  );
  await writeFile(
    "dist/llms.txt",
    [
      "# 荣耀海军 Glory Navy",
      "",
      `> ${publicSiteMetadata().description}`,
      "",
      "这是 EVE Online 玩家军团网站，游戏及舰船素材归 CCP 所有。",
      "",
      "## 公开介绍",
      `- [军团介绍](${publicSiteURL}#strength)：2016 年在晨曦成立，2021 年 8 月在宁静重建。`,
      `- [军团活动](${publicSiteURL}#fleet)：PVP 舰队、PVE 教学、工业生产。`,
      `- [成员保障](${publicSiteURL}#support)：新人帮助、补损与福利、PAP 与果壳币奖励。福利条件与额度以站内当前规则为准。`,
      `- [加入我们](${publicSiteURL}#join)：QQ 招募群 674743797，联系群管理员；入团要求见该章节。`,
      "",
      "本站成员页面需要登录，不属于公开介绍。",
      "",
    ].join("\n"),
  );
  console.log(
    "Public home prerendered; app shell, robots, sitemap and llms.txt generated.",
  );
} finally {
  await server.close();
}
