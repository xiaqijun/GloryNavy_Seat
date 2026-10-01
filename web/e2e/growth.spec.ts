import { test, expect, type Page } from "@playwright/test";
import { mkdirSync } from "node:fs";

async function setup(
  page: Page,
  admin = true,
  simple = false,
  language = "zh-CN",
  multiple = false,
  multipleProjects = false,
) {
  const writes: Record<string, any>[] = [];
  let policies: Record<string, any>[] = simple
    ? [
        {
          corporation_id: "900",
          kind: "growth_fitting_1",
          version: "1",
          config: {
            enabled: true,
            effective_at: "2020-01-01T00:00:00Z",
            project_name: "新兵远程护卫",
            ship_type_id: "587",
            fitting_id: "1",
            skill_plan_id: "0",
            reference_minor: 0,
            day_zone: "Asia/Shanghai",
            note: "",
            rewards: { fittings: [], items: [], coins_minor: 125 },
          },
        },
      ]
    : [];
  if (multipleProjects && policies[0])
    policies.push({
      ...policies[0],
      kind: "growth_fitting_2",
      config: {
        ...policies[0].config,
        project_name: "后勤成长项目",
        ship_type_id: "590",
        fitting_id: "2",
      },
    });
  let reject = true;
  const fits = [
    { id: "1", name: "新兵远程护卫", fit: { ship_type_id: "587" } },
    { id: "2", name: "后勤支援配装", fit: { ship_type_id: "590" } },
  ];

  await page.route("https://images.evetech.net/**", (r) => r.abort());
  await page.route("**/api/v1/**", async (r) => {
    const p = new URL(r.request().url()).pathname;
    let data: unknown = {};
    if (p.endsWith("/modules"))
      data = [
        "system",
        "identity",
        "eve",
        "access",
        "welfare",
        "exchange",
        "fittings",
        "skills",
      ].map((id) => ({ id, api_version: 1, version: "0.1.0" }));
    else if (p === "/api/v1/identity/session")
      data = {
        authenticated: true,
        session: {
          user_id: "user",
          character: { id: "123", name: "Hajimi1" },
          csrf_token: "csrf",
          expires_at: "2099-01-01T00:00:00Z",
        },
      };
    else if (p === "/api/v1/access/me")
      data = {
        administrator: admin,
        can_manage: admin,
        can_manage_sync: admin,
        site_roles: [],
        characters: [],
      };
    else if (p === "/api/v1/welfare/context")
      data = {
        administrator: admin,
        characters: [
          ...(multiple
            ? [
                {
                  id: "456",
                  name: "Alt pilot",
                  account_id: "user",
                  corporation_id: "900",
                },
              ]
            : []),
          {
            id: "123",
            name: "Hajimi1",
            account_id: "user",
            corporation_id: "900",
          },
        ],
        corporations: [{ id: "900", name: "Glory Navy", can_manage: admin }],
        policies,
      };
    else if (p === "/api/v1/exchange/catalog")
      data = {
        items: [
          {
            id: "7",
            version: "3",
            name: "训练礼包",
            archived: false,
            content: {
              fittings: [
                {
                  fitting_id: "1",
                  name: fits[0].name,
                  ship_type_id: "587",
                  corporation_id: "900",
                  quantity: 2,
                },
                {
                  fitting_id: "2",
                  name: fits[1].name,
                  ship_type_id: "590",
                  corporation_id: "900",
                  quantity: 1,
                },
              ],
              items: [{ type_id: "34", name: "三钛合金", quantity: 100 }],
            },
          },
        ],
        next_cursor: "",
      };
    else if (p === "/api/v1/fittings/library") data = fits;
    else if (p === "/api/v1/skills/plans") data = [];
    else if (p === "/api/v1/welfare/growth/check")
      data = {
        state: "met",
        remaining_sp: 0,
        observed_at: "2026-09-20T01:00:00Z",
      };
    else if (p === "/api/v1/welfare/cases")
      data = { items: [], next_cursor: "" };
    else if (p === "/api/v1/welfare/items")
      data = { items: [{ id: "34", name: "三钛合金" }] };
    else if (p === "/api/v1/welfare/commands") {
      const body = r.request().postDataJSON();
      writes.push({ ...body, csrf: r.request().headers()["x-csrf-token"] });
      if (body.action === "profile")
        return r.fulfill({ json: { data: { ...body.profile, version: "2" } } });
      if (reject) {
        reject = false;
        return r.fulfill({
          status: 503,
          json: { error: { message: "保存失败" } },
        });
      }
      if (body.action === "apply")
        return r.fulfill({ json: { data: { id: "1" } } });
      const config = {
        ...body.config,
        project_name: fits[0].name,
        rewards: {
          ...body.config.rewards,
          fittings: body.config.rewards.fittings.map((f: any) => ({
            ...f,
            name: fits.find((v) => v.id === f.fitting_id)?.name,
            ship_type_id: fits.find((v) => v.id === f.fitting_id)?.fit
              .ship_type_id,
          })),
          items: body.config.rewards.items.map((i: any) => ({
            ...i,
            name: "三钛合金",
          })),
        },
      };
      policies = [
        { corporation_id: "900", kind: body.kind, version: "1", config },
      ];
      data = policies[0];
    }
    return r.fulfill({ json: { data } });
  });
  await page.goto(`/welfare?lang=${language}`);
  await page
    .getByRole("button", {
      name: language === "en" ? "Growth benefits" : "成长福利",
      exact: true,
    })
    .click();
  return writes;
}

test("growth projects use fittings and configurable reward combinations", async ({
  page,
}, info) => {
  const writes = await setup(page);
  await expect(page.getByText("暂无成长项目", { exact: true })).toBeVisible();
  await expect(
    page.getByRole("button", { name: "新增项目", exact: true }),
  ).toBeHidden();
  await page.locator("summary").filter({ hasText: "项目管理" }).click();
  await page.getByRole("button", { name: "新增项目", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "新增成长项目" });
  await dialog.getByRole("combobox", { name: "项目舰船配置" }).click();
  await page.getByRole("option", { name: "新兵远程护卫", exact: true }).click();
  await dialog.getByRole("combobox", { name: "实物奖励", exact: true }).click();
  await page.getByRole("option", { name: "训练礼包", exact: true }).click();
  await expect(dialog.getByRole("region", { name: "发放内容" })).toContainText(
    "后勤支援配装",
  );
  // A partially edited amount must not crash the page.
  await dialog.getByRole("spinbutton", { name: "果壳币奖励" }).fill("");
  await expect(
    dialog.getByRole("button", { name: "保存", exact: true }),
  ).toBeDisabled();
  await dialog.getByRole("spinbutton", { name: "果壳币奖励" }).fill("1.25");
  await dialog.getByRole("button", { name: "保存", exact: true }).click();
  await expect(dialog.getByRole("alert")).toHaveText("保存失败");
  await expect(
    dialog.getByRole("spinbutton", { name: "果壳币奖励" }),
  ).toHaveValue("1.25");
  await dialog.getByRole("button", { name: "保存", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect(writes).toHaveLength(2);
  expect(writes[0].request_key).toBe(writes[1].request_key);
  expect(writes[1]).toMatchObject({
    action: "configure",
    kind: "growth_fitting_1",
    csrf: "csrf",
    config: {
      fitting_id: "1",
      reward_id: "7",
      reward_version: "3",
      rewards: {
        coins_minor: 125,
        fittings: [
          { fitting_id: "1", quantity: 2 },
          { fitting_id: "2", quantity: 1 },
        ],
        items: [{ type_id: "34", quantity: 100 }],
      },
    },
  });
  await expect(page.getByRole("region", { name: "发放内容" })).toContainText(
    "新兵远程护卫",
  );
  await page.getByRole("button", { name: "项目配置", exact: true }).click();
  const edit = page.getByRole("dialog", { name: "项目配置" });
  await expect(
    edit.getByRole("combobox", { name: "项目舰船配置" }),
  ).toBeDisabled();
  await expect(
    edit.getByRole("spinbutton", { name: "果壳币奖励" }),
  ).toHaveValue("1.25");
  mkdirSync("../.local/growth/screenshots", { recursive: true });
  for (const width of [1440, 375]) {
    await page.setViewportSize({ width, height: 900 });
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await expect(
      edit.getByRole("button", { name: "保存", exact: true }),
    ).toBeInViewport();
    await page.screenshot({
      path: `../.local/growth/screenshots/editor-${info.project.name}-${width}.png`,
    });
  }
  await edit.getByRole("button", { name: "取消", exact: true }).click();
  await page.getByRole("button", { name: "申请", exact: true }).click();
  await expect(page.getByRole("dialog")).toContainText("后勤支援配装");
  await expect(page.getByRole("dialog")).toContainText("1.25");
});

test("members cannot configure growth rewards", async ({ page }) => {
  await setup(page, false);
  await expect(
    page.getByRole("button", { name: "新增项目", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "项目配置", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "申请", exact: true }),
  ).toHaveCount(0);
});

test("all growth projects are visible and a qualified alternate enables application", async ({
  page,
}) => {
  await setup(page, false, true, "zh-CN", true, true);
  await page.route("**/api/v1/welfare/growth/check?**", (r) => {
    const params = new URL(r.request().url()).searchParams;
    const met =
      params.get("kind") === "growth_fitting_1" &&
      params.get("character_id") === "123";
    return r.fulfill({
      json: {
        data: {
          state: met ? "met" : "missing",
          remaining_sp: met ? 0 : 1000,
          observed_at: null,
        },
      },
    });
  });
  await page.reload();
  await page.getByRole("button", { name: "成长福利", exact: true }).click();
  await expect(page.locator(".welfare-growth-project")).toHaveCount(2);
  await expect(page.getByRole("combobox", { name: "福利类型" })).toHaveCount(0);
  await expect(page.getByRole("combobox", { name: "领取角色" })).toHaveCount(0);
  await expect(page.locator(".welfare-growth-projects")).not.toContainText(
    "Hajimi1",
  );
  const eligible = page
    .locator(".welfare-growth-project")
    .filter({ hasText: "新兵远程护卫" });
  await expect(
    eligible.getByRole("button", { name: "申请", exact: true }),
  ).toBeVisible();
  await expect(
    page
      .locator(".welfare-growth-project")
      .filter({ hasText: "后勤成长项目" })
      .getByRole("button", { name: "申请", exact: true }),
  ).toHaveCount(0);
  await eligible.getByRole("button", { name: "申请", exact: true }).click();
  await expect(
    page.getByRole("dialog").getByRole("combobox", { name: "领取角色" }),
  ).toHaveText("Hajimi1");
});

test("past claims groups accounts under their actual main character", async ({
  page,
}) => {
  await setup(page, true, true);
  await page.route("**/api/v1/welfare/members?**", (r) =>
    r.fulfill({
      json: {
        data: {
          items: [
            {
              id: "456",
              name: "Alt one",
              account_id: "user",
              corporation_id: "900",
              main_character_name: "Hajimi1",
            },
            {
              id: "789",
              name: "Alt two",
              account_id: "user",
              corporation_id: "900",
              main_character_name: "Hajimi1",
            },
            {
              id: "987",
              name: "Other alt",
              account_id: "other",
              corporation_id: "900",
              main_character_name: "Nuter Zero",
            },
          ],
        },
      },
    }),
  );
  await page.route("**/api/v1/welfare/profile?**", (r) =>
    r.fulfill({
      json: {
        data: {
          account_id: "user",
          verified: false,
          history: { growth_fitting_1: "unused" },
          months: [],
          version: "1",
        },
      },
    }),
  );
  await page.locator("summary").filter({ hasText: "项目管理" }).click();
  await page.getByRole("button", { name: "历史领取", exact: true }).click();
  const members = page.getByRole("combobox", { name: "成员", exact: true });
  await expect(members).toHaveText("Hajimi1");
  await members.click();
  await expect(page.getByRole("option")).toHaveCount(2);
  await expect(
    page.getByRole("option", { name: "Hajimi1", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("option", { name: "Nuter Zero", exact: true }),
  ).toBeVisible();
  await page.getByRole("option", { name: "Hajimi1", exact: true }).click();
  const history = page.getByRole("combobox", {
    name: "新兵远程护卫历史资格",
    exact: true,
  });
  await expect(history).toHaveText("未登记");
  await history.click();
  await expect(page.getByRole("option")).toHaveCount(2);
  await expect(
    page.getByRole("option", { name: "历史未领取", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("option", { name: "历史已领取", exact: true }).click();
  await expect(history).toHaveText("历史已领取");
  const modal = page.getByRole("dialog", { name: "历史领取", exact: true });
  await expect(modal.getByLabel("核验依据")).toHaveCount(0);
  const saved = page.waitForRequest((r) => r.url().endsWith("/welfare/commands") && r.method() === "POST");
  await modal.getByRole("button", { name: "保存", exact: true }).click();
  const body = (await saved).postDataJSON();
  expect(body.note).toBe("更新成长福利领取资料");
  expect(body.profile).toMatchObject({ verified: false, months: [], history: { growth_fitting_1: "used" } });
  await expect(modal).toHaveCount(0);
});

test("English growth configuration remains usable on narrow screens", async ({
  page,
}, info) => {
  await setup(page);
  await page.getByRole("button", { name: "切换为英文", exact: true }).click();
  await page
    .getByRole("button", { name: "Growth benefits", exact: true })
    .click();
  await page
    .locator("summary")
    .filter({ hasText: "Project management" })
    .click();
  await page.getByRole("button", { name: "Add project", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Add growth project" });
  await dialog
    .getByRole("combobox", { name: "Project fitting", exact: true })
    .click();
  await page.getByRole("option", { name: "新兵远程护卫", exact: true }).click();
  await expect(
    dialog.getByRole("combobox", { name: "Physical reward", exact: true }),
  ).toBeVisible();
  await page.setViewportSize({ width: 375, height: 900 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await expect(
    dialog.getByRole("button", { name: "Save", exact: true }),
  ).toBeInViewport();
  mkdirSync("../.local/growth/screenshots", { recursive: true });
  await page.screenshot({
    path: `../.local/growth/screenshots/english-${info.project.name}-375.png`,
  });
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
});

for (const language of ["zh-CN", "en"]) {
  test(`growth applies without extra information ${language}`, async ({
    page,
  }, info) => {
    const en = language === "en";
    const writes = await setup(page, false, true, language, en);
    for (const width of [1440, 375, 320]) {
      await page.setViewportSize({ width, height: 900 });
      await expect(
        page
          .locator(".welfare-growth-project")
          .getByRole("button", { name: en ? "Apply" : "申请", exact: true }),
      ).toBeEnabled();
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
      ).toBe(true);
      mkdirSync("../.local/growth-simple-layout", { recursive: true });
      await page.screenshot({
        path: `../.local/growth-simple-layout/${language}-${info.project.name}-${width}.png`,
      });
    }
    await page
      .getByRole("button", { name: en ? "Apply" : "申请", exact: true })
      .click();
    const dialog = page.getByRole("dialog");
    await expect(
      dialog.locator('textarea, input:not([type="hidden"])'),
    ).toHaveCount(0);
    if (en) {
      await dialog
        .getByRole("combobox", { name: "Recipient character" })
        .click();
      await page.getByRole("option", { name: "Hajimi1", exact: true }).click();
    } else {
      await expect(dialog.getByRole("combobox")).toHaveCount(0);
      await expect(dialog).toContainText("Hajimi1");
    }
    await page.setViewportSize({
      width: info.project.name === "mobile" ? 375 : 1280,
      height: 900,
    });
    mkdirSync("../.local/growth/screenshots", { recursive: true });
    await page.screenshot({
      path: `../.local/growth/screenshots/simple-${language}-${info.project.name}.png`,
    });
    const submit = dialog.getByRole("button", {
      name: en ? "Submit application" : "提交申请",
      exact: true,
    });
    await submit.click();
    await expect(dialog.getByRole("alert")).toHaveText(
      en ? "Save failed" : "保存失败",
    );
    await submit.click();
    await expect(dialog).toHaveCount(0);
    expect(writes).toHaveLength(2);
    expect(writes[0].request_key).toBe(writes[1].request_key);
    expect(writes[1]).toMatchObject({
      action: "apply",
      kind: "growth_fitting_1",
      corporation_id: "900",
      csrf: "csrf",
    });
    expect(writes[1].detail).toEqual({ character_id: "123" });
  });
}

for (const language of ["zh-CN", "en"])
  test(
    "growth eligibility gates application " + language,
    async ({ page }, info) => {
      await setup(page, false, true, language, true);
      const en = language === "en";
      let state = "missing";
      await page.route("**/api/v1/welfare/growth/check?**", (r) =>
        r.fulfill({
          json: {
            data: {
              state,
              remaining_sp: state === "missing" ? 1000 : null,
              observed_at: "2026-09-20T01:00:00Z",
            },
          },
        }),
      );
      const openGrowth = async () => {
        await page.reload();
        await page
          .getByRole("button", {
            name: en ? "Growth benefits" : "成长福利",
            exact: true,
          })
          .click();
      };
      await openGrowth();
      const apply = page.getByRole("button", {
        name: en ? "Apply" : "申请",
        exact: true,
      });
      await expect(apply).toHaveCount(0);
      for (const next of ["unknown", "claimed", "pending"]) {
        state = next;
        await openGrowth();
        await expect(apply).toHaveCount(0);
      }
      state = "met";
      await openGrowth();
      await expect(apply).toBeEnabled();
      await apply.click();
      await expect(page.getByRole("dialog").getByRole("status")).toContainText(
        en ? "Qualified" : "达标",
      );
      mkdirSync("../.local/growth-hull/screenshots", { recursive: true });
      for (const width of [1440, 375, 320]) {
        await page.setViewportSize({ width, height: 800 });
        await expect
          .poll(() =>
            page
              .getByRole("dialog")
              .evaluate((el) => el.scrollWidth <= el.clientWidth),
          )
          .toBe(true);
        await page.screenshot({
          path: `../.local/growth-hull/screenshots/${language}-${info.project.name}-${width}.png`,
        });
      }
    },
  );
