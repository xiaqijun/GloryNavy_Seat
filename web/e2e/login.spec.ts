import { test, expect } from "@playwright/test";

test.beforeEach(async ({ page }) => {
  await page.route("**/api/v1/community/profile", (r) =>
    r.fulfill({
      json: {
        data: {
          version: "1",
          complete: true,
          qq: { value: "123456", version: "1", confirmation: "pending" },
          kook: { value: "测试舰长", version: "1", confirmation: "pending" },
        },
      },
    }),
  );
});

test.beforeEach(async ({ page }) => {
  await page.route("**/api/v1/identity/characters", (r) =>
    r.fulfill({
      json: {
        data: {
          characters: [
            { id: "123", name: "测试舰长", status: "active", is_main: true },
          ],
        },
      },
    }),
  );
});

const anonymous = { authenticated: false, session: null };
const member = {
  authenticated: true,
  session: {
    user_id: "test-user",
    character: { id: "123", name: "测试舰长" },
    csrf_token: "browser-csrf",
    expires_at: "2099-01-01T00:00:00Z",
  },
};

test("protected pages return to the public home without a standalone login", async ({
  page,
}) => {
  await page.route("**/api/v1/identity/session", (route) =>
    route.fulfill({ json: { data: anonymous } }),
  );
  await page.goto("/workspace");
  await expect(page).toHaveURL(/\/$/);
  await expect(
    page.getByRole("button", { name: "使用 EVE Online 登录" }),
  ).toBeVisible();
  await expect(page.getByRole("heading", { name: "GloryNavy", exact: true })).toBeVisible();
  await expect(page.locator(".sidebar, .topbar")).toHaveCount(0);
});

test("configured login submits to the server and sanitizes cancellation", async ({
  page,
}) => {
  await page.route("**/api/v1/eve/login-status", (route) =>
    route.fulfill({ json: { data: { configured: true } } }),
  );
  await page.route("**/api/v1/identity/session", (route) =>
    route.fulfill({ json: { data: anonymous } }),
  );
  // Test the browser contract without contacting CCP or inventing real credentials.
  await page.route("**/api/v1/eve/login", async (route) => {
    expect(route.request().method()).toBe("POST");
    expect(route.request().headers().origin).toBe("http://127.0.0.1:5173");
    await route.fulfill({
      status: 303,
      headers: { Location: "/login?error=cancelled" },
    });
  });
  await page.goto("/login");
  await page.getByRole("button", { name: "使用 EVE Online 登录" }).click();
  await expect(page.getByRole("alert")).toHaveText(
    "已取消登录，可以重新尝试。",
  );
  await expect(page).toHaveURL(/\/$/);
  await expect(
    page.getByRole("button", { name: "使用 EVE Online 登录" }),
  ).toBeEnabled();
});

test("signed-in character sees the workspace entry on the public home", async ({
  page,
}) => {
  await page.route("**/api/v1/identity/session", (route) =>
    route.fulfill({ json: { data: member } }),
  );
  await page.goto("/");
  await expect(page.getByRole("link", { name: "进入工作台" })).toHaveAttribute(
    "href",
    "/workspace",
  );
  await expect(page.getByRole("button", { name: "使用 EVE Online 登录" })).toHaveCount(0);
});

test("anonymous account visits return to the public home", async ({
  page,
}) => {
  await page.route("**/api/v1/identity/session", async (r) => {
    await r.fulfill({ json: { data: anonymous } });
  });
  await page.goto("/account");
  await expect(page).toHaveURL(/\/$/);
  await expect(page.getByRole("button", { name: "使用 EVE Online 登录" })).toBeVisible();
  await expect(page.locator(".sidebar, .topbar")).toHaveCount(0);
});
