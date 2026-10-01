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

test("multiple characters preserve login identity, main selection and unlink boundaries", async ({
  page,
}, testInfo) => {
  let main = "101";
  let ids = ["101", "202", "303", "404", "505"];
  let refuseUnlink = true;
  const names: Record<string, string> = {
    "101": "荣耀指挥官",
    "202": "远航工业与后勤支援舰长",
    "303": "矿工",
    "404": "侦察员",
    "505": "运输舰长",
  };
  await page.route("https://images.evetech.net/**", (r) => r.abort());
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
            user_id: "member",
            character: { id: "101", name: names["101"] },
            main_character: { id: main, name: names[main] },
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
          characters: ids.map((id) => ({
            id,
            name: names[id],
            status: "active",
            is_main: main === id,
          })),
        },
      },
    }),
  );
  await page.route("**/api/v1/access/me", (r) =>
    r.fulfill({
      json: {
        data: {
          administrator: false,
          site_roles: [{ id: "r1", name: "报表查看" }],
          characters: ids.map((id) => ({
            character_id: id,
            state: "ready",
            corporation: {
              id: id === "101" ? "10" : "20",
              name: id === "101" ? "荣耀海军" : "后勤军团",
              alliance_id: "0",
              ceo_id: "999",
            },
            roles: id === "101" ? ["Director"] : ["Accountant"],
            roles_at_hq: [],
            roles_at_base: [],
            roles_at_other: [],
            synced_at: new Date().toISOString(),
            valid_until: "2099-01-01T00:00:00Z",
          })),
        },
      },
    }),
  );
  let mainCalls = 0;
  await page.route("**/api/v1/identity/characters/*/main", (r) => {
    expect(r.request().method()).toBe("POST");
    expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
    main = r.request().url().split("/").at(-2)!;
    mainCalls++;
    return r.fulfill({ json: { data: { updated: true } } });
  });
  await page.route("**/api/v1/identity/characters/202", (r) => {
    expect(r.request().method()).toBe("DELETE");
    expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
    if (refuseUnlink)
      return r.fulfill({
        status: 409,
        json: { error: { message: "请刷新后重试" } },
      });
    ids = ["101"];
    return r.fulfill({ json: { data: { updated: true } } });
  });
  await page.goto("/account");
  await expect(
    page.getByRole("heading", { name: names["101"], exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "解绑角色", exact: true }),
  ).toHaveCount(0);
  const roster = page.getByRole("group", { name: "选择查看角色" });
  await expect(roster.getByRole("button")).toHaveCount(5);
  const widths = await roster
    .getByRole("button")
    .evaluateAll((buttons) =>
      buttons.map((button) => button.getBoundingClientRect().width),
    );
  expect(Math.max(...widths)).toBeLessThanOrEqual(220);
  expect(Math.max(...widths)).toBeLessThanOrEqual(200);
  const heights = await roster
    .getByRole("button")
    .evaluateAll((buttons) =>
      buttons.map((button) => button.getBoundingClientRect().height),
    );
  expect(Math.max(...heights) - Math.min(...heights)).toBeLessThan(1);
  await expect(
    page.locator(".account-corporation-card .account-identity-header"),
  ).toContainText(names["101"]);
  await roster.getByRole("button", { name: names["202"] }).click();
  await expect(
    page.getByRole("heading", { name: "后勤军团", exact: true }),
  ).toBeVisible();
  await expect(page.getByText("总监", { exact: true })).toHaveCount(0);
  expect(mainCalls).toBe(0);
  await page.getByRole("button", { name: "设为主角色", exact: true }).click();
  await expect(
    page.getByRole("status").filter({ hasText: "主角色已更新" }),
  ).toBeVisible();
  expect(main).toBe("202");
  await page.reload();
  await expect(
    page.getByRole("heading", { name: names["202"], exact: true }),
  ).toBeVisible();
  await expect(
    roster.getByRole("button", { name: "荣耀指挥官", exact: true }),
  ).toBeVisible();
  await expect(roster.getByText("本次登录", { exact: true })).toHaveCount(0);
  await roster.getByRole("button", { name: "荣耀指挥官", exact: true }).click();
  await page.getByRole("button", { name: "解绑角色", exact: true }).click();
  await expect(page.getByRole("alertdialog")).toContainText(
    "本次登录也会退出。",
  );
  await page.getByRole("button", { name: "取消", exact: true }).click();
  await roster.getByRole("button", { name: names["202"] }).click();
  await page.screenshot({
    path: `../.local/multi-character-${testInfo.project.name}.png`,
    fullPage: true,
  });
  if (testInfo.project.name === "desktop") {
    for (const width of [375, 320]) {
      await page.setViewportSize({ width, height: 800 });
      const mobileHeights = await roster
        .getByRole("button")
        .evaluateAll((buttons) =>
          buttons.map((button) => button.getBoundingClientRect().height),
        );
      expect(
        Math.max(...mobileHeights) - Math.min(...mobileHeights),
      ).toBeLessThan(1);
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
      ).toBe(true);
      await expect(
        page.getByRole("button", { name: "添加角色", exact: true }),
      ).toBeVisible();
      await page.screenshot({
        path: `../.local/multi-character-${width}.png`,
        fullPage: true,
      });
    }
    await page.setViewportSize({ width: 1280, height: 900 });
  }
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await roster.getByRole("button", { name: /荣耀指挥官/ }).click();
  await page.getByRole("button", { name: "设为主角色", exact: true }).click();
  await expect(
    page.getByRole("status").filter({ hasText: "主角色已更新" }),
  ).toBeVisible();
  await roster.getByRole("button", { name: names["202"] }).click();
  await page.getByRole("button", { name: "解绑角色", exact: true }).click();
  const dialog = page.getByRole("alertdialog");
  await expect(
    dialog.getByRole("button", { name: "取消", exact: true }),
  ).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(dialog).not.toBeVisible();
  await page.getByRole("button", { name: "解绑角色", exact: true }).click();
  await dialog.getByRole("button", { name: "确认解绑", exact: true }).click();
  await expect(dialog.getByRole("alert")).toHaveText("请刷新后重试");
  refuseUnlink = false;
  await dialog.getByRole("button", { name: "确认解绑", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  await expect(
    page.getByRole("heading", { name: names["101"], exact: true }),
  ).toBeVisible();
  await expect(page.getByText("报表查看", { exact: true })).toBeVisible();
  await expect(page.getByText(names["202"], { exact: true })).toHaveCount(0);
});

test("link and reauthorize use authenticated explicit endpoints and show callback errors", async ({
  page,
}) => {
  await page.route("**/api/v1/eve/status", (r) =>
    r.fulfill({ json: { data: { configured: true } } }),
  );
  await page.route("**/api/v1/identity/session", (r) =>
    r.fulfill({
      json: {
        data: {
          authenticated: true,
          session: {
            user_id: "member",
            character: { id: "101", name: "舰长" },
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
            { id: "101", name: "舰长", status: "active", is_main: true },
          ],
        },
      },
    }),
  );
  let operation = "";
  await page.route("**/api/v1/eve/characters/**", (r) => {
    expect(r.request().method()).toBe("POST");
    expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
    operation = r.request().url();
    return r.fulfill({
      json: { data: { url: "https://login.eveonline.com/mock-consent" } },
    });
  });
  await page.route("https://login.eveonline.com/mock-consent", (r) =>
    r.fulfill({
      status: 302,
      headers: {
        Location: `http://127.0.0.1:5173/account?error=${operation.endsWith("link") ? "character_conflict" : "wrong_character"}`,
      },
    }),
  );
  await page.goto("/account");
  await page.getByRole("button", { name: "添加角色", exact: true }).click();
  await expect(page.getByRole("alert")).toHaveText(
    "该角色已属于其他本站账号，可使用“合并账号”验证后迁入。",
  );
  expect(operation).toContain("/characters/link");
  await page.getByRole("button", { name: "更新角色授权", exact: true }).click();
  await expect(page.getByRole("alert")).toHaveText(
    "选择的角色不符，请选择需要更新授权的角色。",
  );
  expect(operation).toContain("/characters/101/reauthorize");
  await expect(
    page.getByRole("heading", { name: "舰长", exact: true }),
  ).toBeVisible();
});
