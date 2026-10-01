import { expect, test, type Page } from "@playwright/test";

async function setup(page: Page, canManage = false) {
  let catalogReads = 0;
  await page.route("**/api/v1/**", (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/v1/modules")
      return route.fulfill({
        json: {
          data: [
            "system",
            "identity",
            "eve",
            "access",
            "exchange",
            "approval",
          ].map((id) => ({ id, api_version: 1, version: "0.1.0" })),
        },
      });
    if (path === "/api/v1/identity/session")
      return route.fulfill({
        json: {
          data: {
            authenticated: true,
            session: {
              user_id: "member",
              character: { id: "123", name: "普通成员" },
              csrf_token: "csrf",
              expires_at: "2099-01-01T00:00:00Z",
            },
          },
        },
      });
    if (path === "/api/v1/access/me")
      return route.fulfill({
        json: {
          data: {
            administrator: false,
            can_manage: canManage,
            can_manage_sync: false,
            site_roles: [],
            characters: [],
          },
        },
      });
    if (path === "/api/v1/approval/context")
      return route.fulfill({
        json: {
          data: {
            allowed: false,
            sources: [],
            corporations: [],
            people: [],
            unavailable: [],
          },
        },
      });
    if (path === "/api/v1/exchange/catalog") catalogReads++;
    return route.fulfill({
      status: 403,
      json: { error: { message: "Forbidden" } },
    });
  });
  return () => catalogReads;
}

test("普通成员看不到管理菜单或奖励库操作，直接输入地址也不挂载页面", async ({
  page,
}) => {
  const catalogReads = await setup(page);
  await page.goto("/rewards");
  await expect(page.getByRole("heading", { name: "页面不存在" })).toBeVisible();
  await expect(page.getByRole("button", { name: "新增奖励" })).toHaveCount(0);
  const nav = page.locator("#workspace-navigation");
  for (const name of [
    "奖励库",
    "系统状态",
    "权限管理",
    "成员",
    "ESI 同步",
    "审批中心",
  ])
    await expect(
      nav.getByRole("link", { name, exact: true, includeHidden: true }),
    ).toHaveCount(0);
  expect(catalogReads()).toBe(0);
});

test("权限管理员只看到自己的管理入口", async ({ page }) => {
  await setup(page, true);
  await page.goto("/access");
  const nav = page.locator("#workspace-navigation");
  await expect(
    nav.getByRole("link", { name: "权限管理", includeHidden: true }),
  ).toHaveCount(1);
  for (const name of ["奖励库", "系统状态", "成员", "ESI 同步", "审批中心"])
    await expect(
      nav.getByRole("link", { name, exact: true, includeHidden: true }),
    ).toHaveCount(0);
});

test("空闲时只预热可见菜单的代码，不读取未访问页面数据", async ({ page }) => {
  const catalogReads = await setup(page);
  const requested: string[] = [];
  page.on("request", (request) => requested.push(new URL(request.url()).pathname));
  await page.goto("/workspace");
  await expect(page.getByRole("heading", { name: "工作台" })).toBeVisible();
  await expect
    .poll(() => requested.some((path) => path.includes("/modules/exchange/page")))
    .toBe(true);
  expect(requested.some((path) => path.includes("/modules/exchange/catalog-page"))).toBe(false);
  expect(catalogReads()).toBe(0);
});

test("首次进入成员区时权限与模块目录并行读取", async ({ page }) => {
  await setup(page);
  let releaseCatalog!: () => void;
  const catalogGate = new Promise<void>((resolve) => {
    releaseCatalog = resolve;
  });
  await page.route("**/api/v1/modules", async (route) => {
    await catalogGate;
    await route.fallback();
  });
  let requestedAccess = false;
  page.on("request", (request) => {
    if (new URL(request.url()).pathname === "/api/v1/access/me")
      requestedAccess = true;
  });
  try {
    await page.goto("/workspace");
    await expect.poll(() => requestedAccess).toBe(true);
  } finally {
    releaseCatalog();
  }
  await expect(page.getByRole("heading", { name: "工作台" })).toBeVisible();
});

test("当前页面数据未完成时暂停后台菜单预热", async ({ page }) => {
  await setup(page);
  let releaseData!: () => void;
  const dataGate = new Promise<void>((resolve) => {
    releaseData = resolve;
  });
  await page.route("**/api/v1/identity/characters", async (route) => {
    await dataGate;
    await route.fallback();
  });
  const requested: string[] = [];
  page.on("request", (request) => requested.push(new URL(request.url()).pathname));
  const dataRequested = page.waitForRequest("**/api/v1/identity/characters");
  try {
    await page.goto("/workspace");
    await expect(page.getByRole("heading", { name: "工作台" })).toBeVisible();
    await dataRequested;
    await page.waitForTimeout(1600);
    expect(requested.some((path) => path.includes("/modules/exchange/page"))).toBe(false);
  } finally {
    releaseData();
  }
  await expect
    .poll(() => requested.some((path) => path.includes("/modules/exchange/page")))
    .toBe(true);
});
