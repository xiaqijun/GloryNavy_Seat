import { expect, test, type Page } from "@playwright/test";
import { assertLayout, stateLayouts } from "./state-layout";

async function setup(page: Page, authenticated = true) {
  await page.route("https://images.evetech.net/**", (r) => r.abort());
  await page.route("**/api/v1/**", (r) => {
    const path = new URL(r.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      modules: [
        "system",
        "identity",
        "eve",
        "access",
        "community",
        "market",
      ].map((id) => ({ id, version: "0.1.0", api_version: 1 })),
      "identity/session": {
        authenticated,
        session: authenticated
          ? {
              user_id: "layout-user",
              character: { id: "123", name: "Layout pilot" },
              csrf_token: "test",
              expires_at: "2099-01-01T00:00:00Z",
            }
          : null,
      },
      "identity/characters": {
        characters: [
          { id: "123", name: "Layout pilot", is_main: true, status: "active" },
        ],
      },
      "access/me": {
        administrator: true,
        can_manage: true,
        can_manage_sync: true,
        site_roles: [],
        characters: [],
      },
      "system/status": {
        name: "GloryNavy",
        version: "test",
        environment: "tranquility",
        database: "ready",
        schema_version: 37,
      },
      "eve/status": { configured: true, corporation_roles: true },
      "community/profile": {
        complete: true,
        version: "1",
        qq: { value: "123456", version: "1", confirmation: "pending" },
        kook: { value: "Layout pilot", version: "1", confirmation: "pending" },
      },
      "market/settings": {
        administrator: true,
        settings: { ratio_bps: 10000, version: 1 },
      },
      "eve/sync/characters/123": { resources: [] },
    };
    return r.fulfill({
      json: { data: fixtures[path.replace("/api/v1/", "")] ?? {} },
    });
  });
}

for (const [name, path, endpoint] of [
  ["home", "/", "system/status"],
  ["system", "/system", "system/status"],
  ["market", "/appraisal", "market/settings"],
  ["account", "/account", "identity/characters"],
])
  stateLayouts({
    name,
    setup,
    path,
    endpoint: `**/api/v1/${endpoint}`,
    errorText:
      name === "account"
        ? /角色信息读取失败|Unable to load character|Failed to load character/
        : undefined,
  });

for (const locale of ["zh-CN", "en"])
  test(`public login layout ${locale}: compatibility redirect`, async ({
    page,
  }, info) => {
    await setup(page, false);
    await page.addInitScript(
      (locale) => localStorage.setItem("glorynavy.locale", locale),
      locale,
    );
    await page.route("**/api/v1/identity/session", (r) =>
      r.fulfill({ json: { data: { authenticated: false, session: null } } }),
    );
    await page.goto("/login?error=cancelled");
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    for (const width of [1440, 375, 320]) {
      await page.setViewportSize({ width, height: 568 });
      await assertLayout(page);
    }
    await expect(
      page.getByRole("button", {
        name: /使用 EVE Online 登录|Sign in with EVE Online/,
      }),
    ).toHaveCount(1);
    await page.screenshot({
      path: info.outputPath("login-320.png"),
      fullPage: true,
    });
  });

for (const locale of ["zh-CN", "en"])
  test(`login image delay and failure keep a visible label ${locale}`, async ({
    page,
  }, info) => {
    await setup(page, false);
    await page.addInitScript(
      (locale) => localStorage.setItem("glorynavy.locale", locale),
      locale,
    );
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    await page.route("https://web.ccpgamescdn.com/**", async (route) => {
      await gate;
      await route.abort();
    });
    await page.setViewportSize({ width: 320, height: 568 });
    try {
      await page.route("**/api/v1/identity/session", (r) =>
        r.fulfill({ json: { data: { authenticated: false, session: null } } }),
      );
      await page.goto("/login", { waitUntil: "domcontentloaded" });
      const label =
        locale === "en" ? "Sign in with EVE Online" : "使用 EVE Online 登录";
      const button = page.getByRole("button", { name: label });
      await expect(button.getByText(label, { exact: true })).toBeVisible();
      await expect(button).toBeEnabled();
      await assertLayout(page);
      await page.screenshot({
        path: info.outputPath("login-image-pending-320.png"),
      });
      release();
      await expect(button.locator("img")).toHaveCount(0);
      await expect(button.getByText(label, { exact: true })).toBeVisible();
    } finally {
      release();
    }
  });
