import { mockCharacterSync } from "./sync-fixture";
import { test, expect } from "@playwright/test";

test.beforeEach(async ({ page }) => {
  await mockCharacterSync(page);
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

test("corporation roles stay compact and revoked authorization can be renewed", async ({
  page,
}) => {
  await page.route("**/api/v1/eve/status", (r) =>
    r.fulfill({
      json: { data: { configured: true, corporation_roles: true } },
    }),
  );
  await page.route("**/api/v1/identity/session", (r) =>
    r.fulfill({
      json: {
        data: {
          authenticated: true,
          session: {
            user_id: "test-user",
            character: { id: "123", name: "测试舰长" },
            csrf_token: "test-csrf",
            expires_at: "2099-01-01T00:00:00Z",
          },
        },
      },
    }),
  );
  let state = "ready";
  await page.route("**/api/v1/access/me", (r) =>
    r.fulfill({
      json: {
        data: {
          administrator: false,
          site_roles: [],
          characters: [
            {
              character_id: "123",
              state,
              corporation: {
                id: "10",
                name: "荣耀海军",
                alliance_id: "0",
                ceo_id: "999",
              },
              roles: [
                "Director",
                "Accountant",
                "Auditor",
                "Trader",
                "Diplomat",
                "Factory_Manager",
                "Personnel_Manager",
              ],
              roles_at_hq: ["Hangar_Take_1"],
              roles_at_base: [],
              roles_at_other: [],
              synced_at: new Date().toISOString(),
              valid_until: "2099-01-01T00:00:00Z",
            },
          ],
        },
      },
    }),
  );
  await page.goto("/account");
  await expect(page.getByText("荣耀海军", { exact: true })).toBeVisible();
  await expect(page.getByText("总监", { exact: true })).toBeVisible();
  await expect(page.getByText("人事主管", { exact: true })).not.toBeVisible();
  await page.getByText("更多职务（1）", { exact: true }).click();
  await expect(page.getByText("人事主管", { exact: true })).toBeVisible();
  await expect(page.getByText("本站管理员", { exact: true })).toHaveCount(0);
  await expect(
    page.getByText("机库存取 [1]", { exact: false }),
  ).not.toBeVisible();
  await page.getByText("地点职务", { exact: true }).click();
  await expect(
    page.getByText("机库存取 [1]", { exact: false }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  state = "reauthorize";
  await page.getByRole("button", { name: "同步ESI 授权" }).click();
  await expect(
    page.getByRole("button", { name: "更新 EVE 授权" }),
  ).toBeVisible();
  await expect(page.getByText("总监", { exact: true })).toHaveCount(0);
  await expect(page.getByText("人事主管", { exact: true })).toHaveCount(0);
  await page.route("**/api/v1/eve/characters/123/reauthorize", (r) => {
    expect(r.request().headers()["x-csrf-token"]).toBe("test-csrf");
    return r.fulfill({
      json: { data: { url: "https://login.eveonline.com/mock-consent" } },
    });
  });
  await page.route("https://login.eveonline.com/mock-consent", (r) =>
    r.fulfill({
      status: 302,
      headers: { Location: "http://127.0.0.1:5173/account?error=cancelled" },
    }),
  );
  await page.getByRole("button", { name: "更新 EVE 授权" }).click();
  await expect(page.getByRole("alert")).toHaveText(
    "已取消授权，角色绑定未改变。",
  );
});
