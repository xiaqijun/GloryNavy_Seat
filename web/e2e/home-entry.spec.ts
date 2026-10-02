import { test, expect, type Page } from "@playwright/test";

test("language switching reloads anchored pages in both directions", async ({ page }) => {
  await setup(page, "anonymous");
  let documents = 0;
  page.on("request", request => { if (request.isNavigationRequest() && request.frame() === page.mainFrame()) documents++; });
  await page.goto("/?view=public#strength");
  await expect(page.locator("html")).toHaveAttribute("lang", "zh-CN");
  await page.getByRole("button", {name:"切换为英文"}).click();
  await expect(page.locator("html")).toHaveAttribute("lang", "en");
  await expect(page.getByRole("heading", {name:"More than a single ship."})).toBeVisible();
  await expect(page).toHaveURL(/\?view=public#strength$/);
  await page.getByRole("button", {name:"Switch to Chinese"}).click();
  await expect(page.locator("html")).toHaveAttribute("lang", "zh-CN");
  await expect(page).toHaveURL(/\?view=public#strength$/);
  expect(documents).toBe(3);
});

async function setup(
  page: Page,
  mode: "anonymous" | "signed-in" | "failed" | "system-only",
) {
  let state = mode;
  let statusCalls = 0;
  let sessionCalls = 0;
  let moduleCalls = 0;
  await page.route("https://web.ccpgamescdn.com/**", (route) => route.abort());
  await page.route("**/api/v1/**", (route) => {
    const path = new URL(route.request().url()).pathname;
    const json = (data: unknown) => route.fulfill({ json: { data } });
    if (path === "/api/v1/eve/public/activity") {
      const now = new Date();
      const months = Array.from({ length: 6 }, (_, i) => ({ month: new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth() - 5 + i, 1)).toISOString().slice(0, 7), kills: 100 + i, value: 12340000000 }));
      return json({ corporation_id: 98530802, combat: { months, updated_at: now.toISOString(), stale: false }, online: { characters: 8, covered_characters: 20, bound_characters: 25, updated_at: now.toISOString(), expires_at: new Date(now.getTime()+180000).toISOString() } });
    }
    if (path === "/api/v1/eve/public/corporation")
      return json({
        corporation_id: 98530802,
        name: "Glory Navy",
        ticker: "G.N.V",
        member_count: 379,
        date_founded: "2017-09-27T11:40:29Z",
        updated_at: "2026-09-23T00:00:00Z",
        stale: false,
        alliance: {
          id: 99003581,
          name: "Fraternity.",
          ticker: "FRT",
          corporation_count: 213,
        },
      });
    if (path === "/api/v1/modules") {
      moduleCalls++;
      return json(
        (mode === "system-only"
          ? ["system"]
          : ["system", "identity", "eve"]
        ).map((id) => ({ id, version: "0.1.0", api_version: 1 })),
      );
    }
    if (path === "/api/v1/identity/session") {
      sessionCalls++;
      if (state === "failed")
        return route.fulfill({
          status: 503,
          json: { error: { message: "Unavailable" } },
        });
      return json({
        authenticated: state === "signed-in",
        session:
          state === "signed-in"
            ? {
                user_id: "test-user",
                character: { id: "123", name: "测试角色" },
                csrf_token: "test-only",
                expires_at: "2099-01-01T00:00:00Z",
              }
            : null,
      });
    }
    if (path === "/api/v1/identity/characters") return json({ characters: [] });
    if (path === "/api/v1/eve/login-status") return json({ configured: true });
    if (path === "/api/v1/system/status") {
      statusCalls++;
      return json({
        name: "GloryNavy",
        version: "test",
        environment: "tranquility",
        database: "ready",
        schema_version: 1,
      });
    }
    return route.fulfill({
      status: 404,
      json: { error: { message: "Not found" } },
    });
  });
  return {
    recover: () => {
      state = "anonymous";
    },
    statusCalls: () => statusCalls,
    sessionCalls: () => sessionCalls,
    moduleCalls: () => moduleCalls,
  };
}

test("public home does not read private module or business data", async ({
  page,
}) => {
  const calls: string[] = [];
  page.on("request", (request) => {
    const path = new URL(request.url()).pathname;
    if (path.startsWith("/api/v1/")) calls.push(path);
  });
  await setup(page, "anonymous");
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "GloryNavy", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "使用 EVE Online 登录" }),
  ).toBeVisible();
  await expect(page.locator(".sidebar, .topbar")).toHaveCount(0);
  expect(calls.length).toBeGreaterThan(0);
  expect(
    calls.every((path) =>
      ["/api/v1/identity/session", "/api/v1/eve/public/corporation", "/api/v1/eve/public/activity"].includes(
        path,
      ),
    ),
  ).toBe(true);
  await expect(page.locator(".landing-stats")).toContainText("379");
  await expect(page.locator(".landing-stat dt")).toHaveText(["军团角色", "在线角色", "本月击毁", "击毁价值"]);
  await expect(page.locator(".landing-stat dd").nth(1)).toHaveText("8");
  await expect(page.locator(".landing-combat-chart")).toHaveCount(0);
  await expect(page.locator(".landing-stat dd[title], .landing-stat p")).toHaveCount(0);
});

test("missing activity keeps unknown values and corporation data", async ({ page }) => {
  await setup(page, "anonymous");
  await page.route("**/api/v1/eve/public/activity", route => route.fulfill({json:{data:{corporation_id:98530802,combat:null,online:null}}}));
  await page.goto("/#strength");
  await expect(page.locator(".landing-stat dd")).toHaveText(["379", "—", "—", "—"]);
  await expect(page.locator(".landing-data-footer")).toContainText("公开战绩暂不可用");
  await expect(page.locator(".landing-combat-chart")).toHaveCount(0);
});

test("online values expire without waiting for another successful fetch", async ({ page }) => {
  await setup(page, "anonymous");
  let onlineExpiry = 0;
  await page.route("**/api/v1/eve/public/activity", route => {
    if (!onlineExpiry) onlineExpiry = Date.now() + 4000;
    if (Date.now() >= onlineExpiry) return route.fulfill({status:503,json:{error:{message:"Unavailable"}}});
    return route.fulfill({json:{data:{corporation_id:98530802,combat:null,online:{characters:2,covered_characters:3,bound_characters:4,updated_at:new Date().toISOString(),expires_at:new Date(onlineExpiry).toISOString()}}}});
  });
  await page.goto("/#strength");
  await expect(page.locator(".landing-stat dd").nth(1)).toHaveText("2");
  await expect(page.locator(".landing-stat dd").nth(1)).toHaveText("—", {timeout:8000});
});

test("signed-in visitors keep introduction with a workspace link", async ({
  page,
}) => {
  await setup(page, "signed-in");
  await page.goto("/");
  await expect(page.getByRole("link", { name: "进入工作台" })).toHaveAttribute(
    "href",
    "/workspace",
  );
  await page.getByRole("link", { name: "进入工作台" }).click();
  await expect(
    page.getByRole("heading", { name: "工作台", exact: true }),
  ).toBeVisible();
});

test("session outage does not hide public content or recruitment", async ({
  page,
}) => {
  await setup(page, "failed");
  await page.goto("/");
  await page
    .locator(".landing-hero-copy")
    .getByRole("link", { name: "加入我们" })
    .click();
  await expect(
    page.getByRole("button", { name: "复制 QQ 群号" }),
  ).toBeVisible();
  await expect(page.getByRole("link", { name: "加入 KOOK" })).toHaveAttribute(
    "href",
    "https://kook.vip/h9CYhU",
  );
  await expect(
    page.getByRole("heading", { name: "工作台", exact: true }),
  ).toHaveCount(0);
});

test("anonymous workspace still goes to login", async ({ page }) => {
  await setup(page, "anonymous");
  await page.goto("/workspace");
  await expect(page).toHaveURL(/\/login$/);
  await expect(
    page.getByRole("button", { name: "使用 EVE Online 登录" }),
  ).toBeVisible();
});

test("member navigation reuses fresh session and module catalog", async ({ page }) => {
  const requests = await setup(page, "signed-in");
  await page.goto("/workspace");
  await expect(page.getByRole("heading", { name: "工作台", exact: true })).toBeVisible();
  const initialSessionCalls = requests.sessionCalls();
  const initialModuleCalls = requests.moduleCalls();
  await page.locator('.desk-panel').filter({
    has: page.getByRole("heading", { name: "我的角色" }),
  }).getByRole("link", { name: "查看全部" }).click();
  await expect(page).toHaveURL(/\/account$/);
  await expect(page.getByRole("heading", { name: "我的角色" })).toBeVisible();
  expect(requests.sessionCalls()).toBe(initialSessionCalls);
  expect(requests.moduleCalls()).toBe(initialModuleCalls);
});

test("anonymous approval link does not preload its private page", async ({ page }) => {
  await setup(page, "anonymous");
  const approvalLoads: string[] = [];
  page.on("request", (request) => {
    if (request.url().includes("/modules/approval/page.tsx"))
      approvalLoads.push(request.url());
  });
  await page.goto("/approvals");
  await expect(page).toHaveURL(/\/login$/);
  await expect(
    page.getByRole("button", { name: "使用 EVE Online 登录" }),
  ).toBeVisible();
  expect(approvalLoads).toHaveLength(0);
});

test("public login uses existing POST flow", async ({ page }) => {
  await setup(page, "anonymous");
  await page.route("**/api/v1/eve/login", async (route) => {
    expect(route.request().method()).toBe("POST");
    await route.fulfill({
      status: 303,
      headers: { Location: "/login?error=cancelled" },
    });
  });
  await page.goto("/");
  await page.getByRole("button", { name: "使用 EVE Online 登录" }).click();
  await expect(page).toHaveURL(/\/login\?error=cancelled$/);
});

for (const width of [320, 375, 768, 1440]) {
  test(`public layout ${width}px supports both languages and reduced motion`, async ({
    page,
  }) => {
    await setup(page, "anonymous");
    await page.setViewportSize({ width, height: 960 });
    await page.emulateMedia({ reducedMotion: "reduce" });
    for (const locale of ["zh-CN", "en"]) {
      await page.goto(`/?lang=${locale}`);
      await expect(page.locator(".landing-titan")).toBeVisible();
      await expect(page.locator(".landing-titan")).toHaveJSProperty(
        "naturalWidth",
        1400,
      );
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
      ).toBe(true);
      const box = await page.locator(".landing-login").boundingBox();
      expect(box!.x).toBeGreaterThanOrEqual(0);
      expect(box!.x + box!.width).toBeLessThanOrEqual(width);
      await expect(page.locator("#join h2")).toHaveCSS("opacity", "1");
      await expect(page.locator(".pin-spacer")).toHaveCount(0);
      for (const panel of await page.locator(".landing-chapter-panel").all()) {
        await expect(panel).toBeVisible();
      }
      await expect(
        page.getByRole("heading", { name: "GloryNavy", exact: true }),
      ).toHaveCSS("opacity", "1");
    }
  });
}

test("desktop chapters remain visible through forward and reverse scrolling and clean up on exit", async ({
  page,
}) => {
  await setup(page, "signed-in");
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await page.goto("/");
  await expect(page.locator(".pin-spacer")).toHaveCount(1);
  const scrollChapter = async (progress: number) => {
    await page.evaluate((p) => {
      const spacer = document.querySelector(".pin-spacer")!;
      const start = spacer.getBoundingClientRect().top + scrollY;
      window.scrollTo({
        top: start + innerHeight * 1.7 * p,
        behavior: "instant",
      });
    }, progress);
  };
  for (const [progress, panel] of [
    [0.1, 0],
    [0.48, 1],
    [0.9, 2],
    [0.62, 1],
    [0.48, 1],
    [0.1, 0],
  ]) {
    await scrollChapter(progress);
    const chapter = page.locator(".landing-chapter-panel").nth(panel);
    await expect(chapter).toHaveCSS("visibility", "visible");
    await expect
      .poll(async () =>
        Number(await chapter.evaluate((e) => getComputedStyle(e).opacity)),
      )
      .toBeGreaterThan(0.5);
    expect(
      Math.abs((await page.locator(".landing-chapters").boundingBox())!.y),
    ).toBeLessThan(2);
  }
  await page.evaluate(() => window.scrollTo({ top: 0, behavior: "instant" }));
  await page.getByRole("link", { name: "进入工作台" }).click();
  await expect(page).toHaveURL(/\/workspace$/);
  await expect(page.locator(".pin-spacer")).toHaveCount(0);
});

test("unavailable public data never displays fabricated zero counts", async ({
  page,
}) => {
  await setup(page, "anonymous");
  await page.route("**/api/v1/eve/public/corporation", (route) =>
    route.fulfill({
      status: 503,
      json: {
        error: {
          code: "public_profile_unavailable",
          message: "公开军团资料暂不可用",
        },
      },
    }),
  );
  await page.goto("/");
  await expect(page.locator(".landing-data-footer")).toContainText(
    "公开资料暂不可用",
  );
  await expect(page.locator(".landing-stat dd").first()).toHaveText("—");
  await expect(page.getByRole("link", { name: "加入 KOOK" })).toHaveAttribute(
    "href",
    "https://kook.vip/h9CYhU",
  );
});
