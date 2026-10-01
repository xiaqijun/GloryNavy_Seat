import { test, expect } from "@playwright/test";
import { mockCharacterSync } from "./sync-fixture";
const proof = "11111111-1111-4111-8111-111111111111";
const preview = {
  id: proof,
  source_main: { id: "201", name: "迁入账号" },
  target_main: { id: "101", name: "保留账号" },
  characters: [
    { id: "201", name: "后勤与工业支援舰长" },
    { id: "202", name: "侦察小号" },
  ],
  token: "a".repeat(64),
  completed: false,
  data: {
    attendance: { points: 12, entries: 4 },
    exchange: { coins_minor: 3500, target_coins_minor: 1500, orders: 2 },
    fittings: { drafts: 1 },
  },
};
test.beforeEach(async ({ page }) => {
  await mockCharacterSync(page);
  await page.route("https://images.evetech.net/**", (r) => r.abort());
  await page.route("**/api/v1/modules", (r) =>
    r.fulfill({
      json: {
        data: ["system", "identity", "eve", "access"].map((id) => ({
          id,
          api_version: 1,
          version: "0.1.0",
        })),
      },
    }),
  );
  await page.route("**/api/v1/eve/status", (r) =>
    r.fulfill({
      json: { data: { configured: true, corporation_roles: false } },
    }),
  );
  await page.route("**/api/v1/identity/session", (r) =>
    r.fulfill({
      json: {
        data: {
          authenticated: true,
          session: {
            user_id: "target",
            character: { id: "101", name: "保留账号" },
            main_character: { id: "101", name: "保留账号" },
            csrf_token: "csrf",
            expires_at: "2099-01-01T00:00:00Z",
          },
        },
      },
    }),
  );
  await page.route("**/api/v1/identity/characters", (r) =>
    r.fulfill({
      json: {
        data: {
          characters: [
            { id: "101", name: "保留账号", status: "active", is_main: true },
          ],
        },
      },
    }),
  );
  await page.route("**/api/v1/access/me", (r) =>
    r.fulfill({
      json: {
        data: {
          administrator: false,
          can_manage: false,
          can_manage_sync: false,
          site_roles: [],
          characters: [],
        },
      },
    }),
  );
  await page.route("**/api/v1/community/profile", (r) =>
    r.fulfill({
      json: {
        data: {
          version: "1",
          complete: true,
          qq: { value: "123456", version: "1", confirmation: "pending" },
          kook: { value: "昵称", version: "1", confirmation: "pending" },
        },
      },
    }),
  );
});
test("merge requires verification, previews totals, and confirms only after review", async ({
  page,
}, info) => {
  let writes = 0;
  await page.route(`**/api/v1/identity/merges/${proof}`, async (r) => {
    if (r.request().method() === "POST") {
      writes++;
      expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
      expect(r.request().postDataJSON()).toEqual({ token: preview.token });
      await r.fulfill({ json: { data: { ...preview, completed: true } } });
      return;
    }
    await r.fulfill({ json: { data: preview } });
  });
  await page.goto("/account");
  await page.getByRole("button", { name: "合并账号", exact: true }).click();
  const start = page.getByRole("dialog");
  await expect(
    start.getByRole("button", { name: "验证另一个账号" }),
  ).toBeVisible();
  await expect(start.getByRole("button", { name: "确认合并" })).toHaveCount(0);
  await page.keyboard.press("Escape");
  await expect(start).toHaveCount(0);
  await page.goto("/account?merge=" + proof);
  const dialog = page.getByRole("dialog");
  await expect(
    dialog.getByText("后勤与工业支援舰长", { exact: true }),
  ).toBeVisible();
  await expect(dialog.getByText("50", { exact: true })).toBeVisible();
  expect(writes).toBe(0);
  for (const width of [1440, 375, 320]) {
    await page.setViewportSize({ width, height: 900 });
    await expect(dialog).toBeVisible();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
  }
  await page.screenshot({
    path: info.outputPath("merge-preview.png"),
    fullPage: true,
  });
  await dialog.getByRole("button", { name: "确认合并" }).click();
  await expect(page).toHaveURL(/\/account\?merged=1$/);
  expect(writes).toBe(1);
  await expect(
    page.getByRole("status").filter({ hasText: "账号已合并" }),
  ).toBeVisible();
});
test("stale proof remains unchanged and cancellation clears verification", async ({
  page,
}) => {
  let deleted = 0;
  await page.route(`**/api/v1/identity/merges/${proof}`, async (r) => {
    if (r.request().method() === "DELETE") {
      deleted++;
      await r.fulfill({ json: { data: { cancelled: true } } });
      return;
    }
    if (r.request().method() === "POST") {
      await r.fulfill({
        status: 409,
        json: { error: { message: "账号资料已变化，请刷新预览" } },
      });
      return;
    }
    await r.fulfill({ json: { data: preview } });
  });
  await page.goto("/account?merge=" + proof);
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("button", { name: "确认合并" }).click();
  await expect(dialog.getByRole("alert")).toContainText("账号资料已变化");
  await dialog.getByRole("button", { name: "取消", exact: true }).click();
  expect(deleted).toBe(1);
  await expect(page).toHaveURL(/\/account$/);
});
