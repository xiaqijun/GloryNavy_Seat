import { test, expect, type Page } from "@playwright/test";

async function setup(page: Page, en = false, admin = true) {
  await page.addInitScript(
    (en) => localStorage.setItem("glorynavy.locale", en ? "en" : "zh-CN"),
    en,
  );
  await page.route("https://images.evetech.net/**", (r) => r.abort());
  let settings = {
    enabled: false,
    corporation_ids: [] as string[],
    corporation_options: [
      { id: "10", name: "Glory Navy" },
      {
        id: "30",
        name: "LongCorporationNameWithoutSpacesABCDEFGHIJKLMNOPQRSTUVWXYZ",
      },
    ],
    version: "1",
    next_run_at: null as string | null,
    run_started_at: null,
    last_run_at: null,
    removed_count: 0,
    skipped_count: 0,
    last_removed_count: 0,
    last_skipped_count: 0,
  };
  let saved: unknown;
  let cleanupCalls = 0;
  let failSave = false;
  await page.route("**/api/v1/**", (r) => {
    const path = new URL(r.request().url()).pathname;
    const json = (data: unknown) => r.fulfill({ json: { data } });
    if (path === "/api/v1/modules")
      return json(
        ["system", "identity", "eve", "access"].map((id) => ({
          id,
          version: "0.1.0",
          api_version: 1,
        })),
      );
    if (path === "/api/v1/identity/session")
      return json({
        authenticated: true,
        session: {
          user_id: "00000000-0000-4000-8000-000000000001",
          character: { id: "101", name: "Admin" },
          csrf_token: "test-only",
          expires_at: "2099-01-01T00:00:00Z",
        },
      });
    if (path === "/api/v1/access/me")
      return json({
        administrator: admin,
        can_manage: true,
        can_manage_sync: true,
        site_roles: [],
        characters: [],
      });
    if (path === "/api/v1/access/members") return json({ items: [], next: "" });
    if (path.includes("corporation-cleanup")) {
      cleanupCalls++;
      if (r.request().method() !== "GET")
        expect(r.request().headers()["x-csrf-token"]).toBe("test-only");
      if (path.endsWith("/preview"))
        return json({
          remove_count: 101,
          keep_count: 6,
          unknown_count: 2,
          items: Array.from({ length: 100 }, (_, i) => ({
            character_id: String(1000 + i),
            name: i
              ? `Pilot ${i}`
              : "LongCharacterNameWithoutSpacesABCDEFGHIJKLMNOPQRSTUVWXYZ",
            corporation_id: "20",
            corporation_name:
              "VeryLongExternalCorporationNameABCDEFGHIJKLMNOPQRSTUVWXYZ",
          })),
        });
      if (r.request().method() === "PUT") {
        if (failSave)
          return r.fulfill({
            status: 409,
            json: { error: { message: "清理规则已更新，请重新读取" } },
          });
        saved = r.request().postDataJSON();
        settings = {
          ...settings,
          ...r.request().postDataJSON(),
          version: "2",
          next_run_at: "2026-10-24T10:00:00Z",
        };
      }
      return json(settings);
    }
    return r.fulfill({
      status: 404,
      json: { error: { message: "Unknown fixture route" } },
    });
  });
  return {
    saved: () => saved,
    calls: () => cleanupCalls,
    fail: () => {
      failSave = true;
    },
  };
}

for (const en of [false, true])
  test(`cleanup preview and save ${en ? "English" : "Chinese"}`, async ({
    page,
  }, info) => {
    const fixture = await setup(page, en);
    await page.goto("/members");
    const trigger = page.getByRole("button", {
      name: en ? "Cleanup rules" : "清理规则",
      exact: true,
    });
    await trigger.click();
    const dialog = page.getByRole("dialog");
    const save = dialog.getByRole("button", {
      name: en ? "Save" : "保存",
      exact: true,
    });
    await dialog
      .getByRole("checkbox", {
        name: en ? "Automatic weekly cleanup" : "每周自动清理",
      })
      .check();
    await expect(save).toBeDisabled();
    await dialog.getByRole("searchbox").fill("glory");
    await expect(
      dialog.getByRole("button", { name: /LongCorporationName/ }),
    ).toHaveCount(0);
    await dialog
      .getByRole("button", {
        name: en ? "Add corporation: Glory Navy" : "添加军团：Glory Navy",
      })
      .click();
    await dialog
      .getByRole("button", {
        name: /^(Add corporation: |添加军团：)LongCorporationName/,
      })
      .click();
    await expect(
      dialog.getByRole("button", {
        name: en ? "Remove corporation: Glory Navy" : "移除军团：Glory Navy",
      }),
    ).toBeVisible();
    await dialog
      .getByRole("button", {
        name: en ? "Preview cleanup" : "预览清理",
        exact: true,
      })
      .click();
    await expect(
      dialog.getByText(
        en
          ? "To remove 101 · Retain 6 · Uncertain 2"
          : "预计清理 101 · 保留 6 · 待核实 2",
      ),
    ).toBeVisible();
    await expect(
      dialog.getByText(
        en ? "Showing the first 100 characters" : "仅展示前 100 个角色",
      ),
    ).toBeVisible();
    await expect(save).toBeInViewport();
    const bounds = await dialog.boundingBox();
    expect(bounds!.x).toBeGreaterThanOrEqual(0);
    expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(
      page.viewportSize()!.width,
    );
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: info.outputPath(`cleanup-${en ? "en" : "zh"}.png`),
      fullPage: true,
    });
    await save.click();
    await expect(dialog).not.toBeVisible();
    expect(fixture.saved()).toEqual({
      enabled: true,
      corporation_ids: ["10", "30"],
      version: "1",
    });
    await expect(trigger).toBeFocused();
    await trigger.click();
    await expect(dialog.getByRole("checkbox")).toBeChecked();
    // The shared close icon opens a tooltip on focus; move into the form
    // before testing the dialog's Escape behavior.
    await dialog.getByRole("searchbox").focus();
    await page.keyboard.press("Escape");
    await expect(dialog).not.toBeVisible();
  });

test("cleanup errors retain draft and ordinary managers have no entry", async ({
  page,
}) => {
  const fixture = await setup(page);
  await page.goto("/members");
  await page.getByRole("button", { name: "清理规则", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("button", { name: "添加军团：Glory Navy" }).click();
  fixture.fail();
  await dialog.getByRole("button", { name: "保存", exact: true }).click();
  await expect(dialog.getByRole("alert")).toContainText("清理规则已更新");
  await expect(
    dialog.getByRole("button", { name: "移除军团：Glory Navy" }),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await page.unrouteAll();
  const ordinary = await setup(page, false, false);
  await page.reload();
  await expect(page.getByText("仅站点管理员可查看成员数据")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "清理规则", exact: true }),
  ).toHaveCount(0);
  expect(ordinary.calls()).toBe(0);
});
