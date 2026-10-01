import { mockCharacterSync } from "./sync-fixture";
import { test, expect } from "@playwright/test";

test("QQ and KOOK details are shared, editable and separate from confirmation", async ({
  page,
}, info) => {
  await mockCharacterSync(page);
  let p = {
    version: "0",
    complete: false,
    qq: { value: "", version: "0", confirmation: "unfilled" },
    kook: { value: "", version: "0", confirmation: "unfilled" },
  };
  let writes = 0;
  let conflict = false;
  await page.route("https://images.evetech.net/**", (r) => r.abort());
  await page.route("**/api/v1/identity/session", (r) =>
    r.fulfill({
      json: {
        data: {
          authenticated: true,
          session: {
            user_id: "member",
            character: { id: "101", name: "主舰长" },
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
            { id: "101", name: "主舰长", status: "active", is_main: true },
            { id: "202", name: "工业小号", status: "active", is_main: false },
          ],
        },
      },
    }),
  );
  await page.route("**/api/v1/eve/status", (r) =>
    r.fulfill({ json: { data: { configured: true } } }),
  );
  await page.route("**/api/v1/community/profile", async (r) => {
    if (r.request().method() === "GET") return r.fulfill({ json: { data: p } });
    expect(r.request().method()).toBe("PUT");
    expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
    writes++;
    const body = r.request().postDataJSON();
    expect(Object.keys(body).sort()).toEqual([
      "kook_name",
      "qq_number",
      "version",
    ]);
    if (conflict || body.version !== p.version)
      return r.fulfill({
        status: 409,
        json: { error: { message: "资料已在其他页面更新，请重新读取后修改" } },
      });
    p = {
      version: String(Number(p.version) + 1),
      complete: true,
      qq: {
        value: body.qq_number,
        version: String(
          Number(p.qq.version) + (body.qq_number !== p.qq.value ? 1 : 0),
        ),
        confirmation:
          body.qq_number !== p.qq.value ? "pending" : p.qq.confirmation,
      },
      kook: {
        value: body.kook_name,
        version: String(
          Number(p.kook.version) + (body.kook_name !== p.kook.value ? 1 : 0),
        ),
        confirmation:
          body.kook_name !== p.kook.value ? "pending" : p.kook.confirmation,
      },
    };
    return r.fulfill({ json: { data: p } });
  });
  await page.goto("/account");
  const qq = page.getByLabel("QQ 号", { exact: true });
  const kook = page.getByLabel("KOOK 昵称", { exact: true });
  await expect(qq).toBeVisible();
  await page.getByRole("button", { name: "保存资料", exact: true }).click();
  await expect(qq).toBeFocused();
  await expect(qq).toHaveAttribute("aria-invalid", "true");
  expect(writes).toBe(0);
  await qq.fill("012345");
  await kook.fill("荣耀后勤 舰长🚀");
  await page.getByRole("button", { name: "保存资料", exact: true }).click();
  expect(writes).toBe(0);
  await qq.fill("123456789");
  await page.screenshot({
    path: `../.local/community-form-${info.project.name}.png`,
    fullPage: true,
  });
  await page.getByRole("button", { name: "保存资料", exact: true }).click();
  await expect(page.getByText("资料已保存", { exact: true })).toBeVisible();
  await expect(
    page
      .locator(".community-binding")
      .filter({ has: page.getByRole("heading", { name: "QQ", exact: true }) })
      .getByText("待确认", { exact: true }),
  ).toBeVisible();
  await expect(
    page
      .locator(".community-binding")
      .filter({ has: page.getByRole("heading", { name: "KOOK", exact: true }) })
      .getByText("待确认", { exact: true }),
  ).toBeVisible();
  await expect(qq).toHaveCount(0);
  const cardLayout = await page.locator(".community-card").evaluate((card) => {
    const rows = card.querySelectorAll(".community-binding");
    return {
      width: card.getBoundingClientRect().width,
      stacked:
        rows[1].getBoundingClientRect().top >=
        rows[0].getBoundingClientRect().bottom,
    };
  });
  expect(cardLayout.width).toBeLessThanOrEqual(360);
  expect(cardLayout.stacked).toBe(true);
  await page
    .getByRole("group", { name: "选择查看角色" })
    .getByRole("button", { name: "工业小号" })
    .click();
  await expect(page.getByText("123456789", { exact: true })).toBeVisible();
  await expect(
    page.getByText("荣耀后勤 舰长🚀", { exact: true }),
  ).toBeVisible();
  p = {
    ...p,
    qq: { ...p.qq, confirmation: "confirmed" },
    kook: { ...p.kook, confirmation: "confirmed" },
  };
  await page.reload();
  await page.getByRole("button", { name: "修改社区资料", exact: true }).click();
  await kook.fill("新的 KOOK 昵称");
  await expect(
    page.getByText("修改后，对应平台需要重新确认。", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "保存资料", exact: true }).click();
  await expect(
    page
      .locator(".community-binding")
      .filter({ has: page.getByRole("heading", { name: "QQ", exact: true }) })
      .getByText("已确认", { exact: true }),
  ).toBeVisible();
  await expect(
    page
      .locator(".community-binding")
      .filter({ has: page.getByRole("heading", { name: "KOOK", exact: true }) })
      .getByText("待确认", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "修改社区资料", exact: true }).click();
  await qq.fill("987654321");
  conflict = true;
  await page.getByRole("button", { name: "保存资料", exact: true }).click();
  await expect(page.getByRole("alert")).toBeFocused();
  await expect(qq).toHaveValue("987654321");
  p = { ...p, version: "3", kook: { ...p.kook, value: "另一页面保存的昵称" } };
  await page.getByRole("button", { name: "读取最新资料", exact: true }).click();
  await expect(kook).toHaveValue("另一页面保存的昵称");
  conflict = false;
  await qq.fill("987654321");
  await page.getByRole("button", { name: "保存资料", exact: true }).click();
  await expect(page.getByText("987654321", { exact: true })).toBeVisible();
  await expect(
    page
      .locator(".community-binding")
      .filter({ has: page.getByRole("heading", { name: "QQ", exact: true }) })
      .getByText("待确认", { exact: true }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: `../.local/community-saved-${info.project.name}.png`,
    fullPage: true,
  });
  if (info.project.name === "desktop") {
    for (const width of [375, 320]) {
      await page.setViewportSize({ width, height: 800 });
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
      ).toBe(true);
      await page.screenshot({
        path: `../.local/community-${width}.png`,
        fullPage: true,
      });
    }
    await page
      .getByRole("button", { name: "修改社区资料", exact: true })
      .click();
    await expect(qq).toBeVisible();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
  }
});

test("community loading failure is recoverable and edits survive server errors", async ({
  page,
}) => {
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
  await page.route("**/api/v1/eve/status", (r) =>
    r.fulfill({ json: { data: { configured: true } } }),
  );
  let fail = true;
  await page.route("**/api/v1/community/profile", (r) =>
    fail || r.request().method() === "PUT"
      ? r.fulfill({
          status: 503,
          json: { error: { message: "保存失败，请稍后重试" } },
        })
      : r.fulfill({
          json: {
            data: {
              version: "0",
              complete: false,
              qq: { value: "", version: "0", confirmation: "unfilled" },
              kook: { value: "", version: "0", confirmation: "unfilled" },
            },
          },
        }),
  );
  await page.goto("/account");
  await expect(
    page.getByText("社区资料读取失败，请重试。", { exact: true }),
  ).toBeVisible();
  fail = false;
  await page
    .getByRole("button", { name: "重新读取社区资料", exact: true })
    .click();
  await page.getByLabel("QQ 号", { exact: true }).fill("123456");
  await page.getByLabel("KOOK 昵称", { exact: true }).fill("昵称");
  await page.getByRole("button", { name: "保存资料", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("保存失败，请稍后重试");
  await expect(page.getByLabel("KOOK 昵称", { exact: true })).toHaveValue(
    "昵称",
  );
});
