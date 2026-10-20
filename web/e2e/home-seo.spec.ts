import { test, expect, type Page } from "@playwright/test";

async function publicAPI(page: Page, signedIn = false) {
  await page.route("**/api/v1/**", (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/v1/identity/session")
      return route.fulfill({
        json: {
          data: {
            authenticated: signedIn,
            session: signedIn
              ? {
                  user_id: "seo-test",
                  character: { id: "123", name: "test" },
                  csrf_token: "test-only",
                  expires_at: "2099-01-01T00:00:00Z",
                }
              : null,
          },
        },
      });
    if (path === "/api/v1/eve/public/corporation")
      return route.fulfill({
        json: {
          data: {
            corporation_id: 98530802,
            name: "Glory Navy",
            ticker: "G.N.V",
            member_count: 379,
            date_founded: "2017-09-27T00:00:00Z",
            updated_at: new Date().toISOString(),
            stale: false,
          },
        },
      });
    if (path === "/api/v1/eve/public/activity")
      return route.fulfill({
        json: {
          data: { corporation_id: 98530802, combat: null, online: null },
        },
      });
    if (path === "/api/v1/modules")
      return route.fulfill({
        json: { data: [{ id: "system", version: "test", api_version: 1 }] },
      });
    if (path === "/api/v1/eve/login-status")
      return route.fulfill({ json: { data: { configured: true } } });
    return route.fulfill({
      status: 401,
      json: { error: { message: "Unauthorized" } },
    });
  });
}

test("initial HTTP document contains public content, one canonical and linked entity data", async ({
  request,
}) => {
  const response = await request.get("/");
  expect(response.status()).toBe(200);
  const html = await response.text();
  expect(html).toContain('data-prerendered="zh-CN"');
  expect(html).toContain("QQ 招募群");
  expect(html).toContain("674743797");
  expect(html).toContain("2016");
  expect(html).toContain("2021.08");
  expect(html).toContain("福利条件与额度以站内当前规则为准。");
  expect(html).not.toContain('id="boot-fallback"');
  expect(html).not.toContain("noindex");
  expect(html.match(/<h1\b/g)).toHaveLength(1);
  expect(html.match(/rel="canonical"/g)).toHaveLength(1);
  expect(html).toContain('href="https://seat.glorynavy.com/"');
  const json = html.match(
    /<script[^>]+type="application\/ld\+json"[^>]*>(.*?)<\/script>/s,
  )![1];
  const graph = JSON.parse(json)["@graph"];
  expect(graph.map((entry: { "@type": string }) => entry["@type"])).toEqual([
    "Organization",
    "WebSite",
    "WebPage",
  ]);
  expect(graph[0].sameAs).toEqual([
    "https://zkillboard.com/corporation/98530802/",
  ]);
  expect(graph[2].about["@id"]).toBe(graph[0]["@id"]);
  expect(graph[0]).not.toHaveProperty("member");
  expect(graph[0]).not.toHaveProperty("foundingDate");
});

test("crawl documents are real files and only advertise the public homepage", async ({
  request,
}) => {
  const robots = await request.get("/robots.txt");
  expect(robots.headers()["content-type"]).toContain("text/plain");
  expect(await robots.text()).toContain("Allow: /$");
  expect(await robots.text()).toContain("Disallow: /");
  const sitemap = await request.get("/sitemap.xml");
  expect(sitemap.headers()["content-type"]).toContain("xml");
  const xml = await sitemap.text();
  expect(xml.match(/<loc>/g)).toHaveLength(1);
  expect(xml).toContain("<loc>https://seat.glorynavy.com/</loc>");
  expect(xml).not.toMatch(/workspace|account|lastmod/);
  const summary = await request.get("/llms.txt");
  expect(summary.headers()["content-type"]).toContain("text/plain");
  expect(await summary.text()).toContain("福利条件与额度以站内当前规则为准");
  expect(await summary.text()).not.toContain("<html");
});

test("public introduction remains usable with JavaScript disabled", async ({
  browser,
  baseURL,
}) => {
  const context = await browser.newContext({
    javaScriptEnabled: false,
    viewport: { width: 375, height: 812 },
  });
  const page = await context.newPage();
  await page.goto(`${baseURL}/#join`);
  await expect(
    page.getByRole("heading", { name: "加入荣耀海军", exact: true }),
  ).toBeVisible();
  await expect(page.locator(".landing-group-number")).toContainText(
    "674743797",
  );
  await expect(page.locator(".landing-requirements li")).toHaveCount(5);
  await expect(
    page.locator('form[action="/api/v1/eve/login"]'),
  ).toHaveAttribute("method", "post");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await context.close();
});

test("hydration keeps the document and metadata coherent, including English", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await publicAPI(page);
  await page.goto("/#strength");
  await expect(page.locator("html")).toHaveAttribute("data-app-ready", "true");
  await expect(page.locator(".landing-stat dd").first()).toHaveText("379");
  await expect(page).toHaveTitle(/荣耀海军.*EVE Online/);
  await expect(page.locator('head meta[name="robots"]')).toHaveCount(1);
  await expect(page.locator('head link[rel="canonical"]')).toHaveCount(1);
  await expect(
    page.locator('head script[type="application/ld+json"]'),
  ).toHaveCount(1);
  expect(errors).toEqual([]);
  await page.goto("/?lang=en#join");
  await expect(page).toHaveTitle(
    "Glory Navy | EVE Online corporation recruitment & member services",
  );
  await expect(page.locator('meta[property="og:locale"]')).toHaveAttribute(
    "content",
    "en_US",
  );
  await expect(page.locator('meta[name="description"]')).toHaveAttribute(
    "content",
    /Tranquility/,
  );
  await expect(
    page.getByRole("heading", { name: "Join Glory Navy", exact: true }),
  ).toBeVisible();
  expect(errors).toEqual([]);
});

test("member documents and client navigation do not inherit public indexing", async ({
  page,
  request,
}) => {
  const response = await request.get("/workspace", {
    headers: { Accept: "text/html" },
  });
  const html = await response.text();
  expect(html).toContain('content="noindex, nofollow"');
  expect(html).not.toContain("application/ld+json");
  expect(html).not.toContain('data-prerendered="zh-CN"');
  await publicAPI(page, true);
  await page.goto("/");
  await page.getByRole("link", { name: "进入工作台" }).click();
  await expect(page).not.toHaveURL(/\/$/);
  await expect(page.locator('meta[name="robots"]')).toHaveAttribute(
    "content",
    "noindex, nofollow",
  );
  await expect(
    page.locator(
      'link[rel="canonical"], meta[property^="og:"], script[type="application/ld+json"]',
    ),
  ).toHaveCount(0);
});

test("OAuth errors still render without a hydration mismatch", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await publicAPI(page);
  await page.goto("/?error=invalid_state");
  await expect(page.locator(".landing-login-error")).toContainText(
    "登录请求已过期",
  );
  expect(errors).toEqual([]);
});
