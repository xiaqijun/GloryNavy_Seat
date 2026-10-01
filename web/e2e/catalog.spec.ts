import { stateLayouts, assertLayout } from "./state-layout";
import { test, expect, type Page } from "@playwright/test";
import { mkdirSync } from "node:fs";

test("English reward library editor supports ISK but excludes site coins", async ({
  page,
}, info) => {
  await setup(page);
  await page.goto("/rewards");
  await page.getByRole("button", { name: "切换为英文", exact: true }).click();
  await page.getByRole("button", { name: "Add reward", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await expect(
    dialog.getByRole("textbox", { name: "Reward name", exact: true }),
  ).toBeVisible();
  await expect(
    dialog.getByText("Nutshell Coin reward", { exact: true }),
  ).toHaveCount(0);
  await page.setViewportSize({ width: 375, height: 900 });
  await expect(
    dialog.getByRole("button", { name: "Save", exact: true }),
  ).toBeInViewport();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  mkdirSync("../.local/catalog", { recursive: true });
  await page.screenshot({
    path: `../.local/catalog/english-${info.project.name}.png`,
  });
});
async function setup(page: Page) {
  const writes: any[] = [];
  let records: any[] = [];
  let fail = true;
  const fit = {
    id: "1",
    corporation_id: "900",
    name: "训练护卫",
    description: "",
    version: "1",
    updated_at: "2026-09-19T00:00:00Z",
    ship_name: "裂谷",
    group: "护卫舰",
    eft: "",
    fit: {
      name: "训练护卫",
      ship_type_id: "587",
      skill_mode: "all5",
      items: [],
    },
  };
  await page.route("https://images.evetech.net/**", (r) => r.abort());
  await page.route("**/api/v1/**", async (r) => {
    const p = new URL(r.request().url()).pathname;
    let data: any = {};
    if (p.endsWith("/modules"))
      data = [
        "system",
        "identity",
        "eve",
        "access",
        "exchange",
        "fittings",
      ].map((id) => ({ id, api_version: 1, version: "0.1.0" }));
    else if (p.endsWith("/identity/session"))
      data = {
        authenticated: true,
        session: {
          user_id: "user",
          character: { id: "123", name: "Test" },
          csrf_token: "csrf",
          expires_at: "2099-01-01T00:00:00Z",
        },
      };
    else if (p.endsWith("/access/me"))
      data = {
        administrator: true,
        can_manage: true,
        can_manage_sync: true,
        site_roles: [],
        characters: [],
      };
    else if (p === "/api/v1/fittings/library/context")
      data = {
        can_manage: true,
        characters: [],
        corporations: [
          { id: "900", name: "Glory Navy", can_create_skills: true },
        ],
      };
    else if (p === "/api/v1/fittings/library") data = [fit];
    else if (p.endsWith("/valuation"))
      data = {
        version: "1",
        isk_value: 10000,
        mid: "10000.00",
        complete: true,
        missing_types: 0,
        observed_at: null,
      };
    else if (p === "/api/v1/exchange/rewards/types")
      data = [{ id: "34", name: "三钛合金" }];
    else if (p === "/api/v1/exchange/catalog") {
      if (r.request().method() === "POST") {
        const b = r.request().postDataJSON();
        writes.push(b);
        expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
        if (fail) {
          fail = false;
          return r.fulfill({
            status: 503,
            json: { error: { message: "保存失败" } },
          });
        }
        const content = {
          fittings: b.content.fittings.map((f: any) => ({
            ...f,
            name: fit.name,
            ship_type_id: "587",
            corporation_id: "900",
            version: "1",
          })),
          items: b.content.items.map((i: any) => ({ ...i, name: "三钛合金" })),
        };
        records = [{ ...b, id: "5", version: "1", content }];
        data = { id: "5" };
      } else data = { items: records, next_cursor: "" };
    } else if (p === "/api/v1/exchange/context")
      data = { characters: [{ id: "123", name: "Test" }], sources: [] };
    else if (
      p === "/api/v1/exchange/wallet" ||
      p === "/api/v1/exchange/rewards/orders"
    )
      data = { items: [], next_cursor: "" };
    else if (p === "/api/v1/exchange/rewards")
      data = {
        admin: true,
        isk_per_coin: 10000,
        version: "1",
        earned_minor: 100,
        reserved_minor: 0,
        spent_minor: 0,
        available_minor: 100,
        next_cursor: "",
        rewards: records.map((x) => ({
          id: x.id,
          version: "1",
          name: x.name,
          type_id: "587",
          quantity: 1,
          isk_value: 10000,
          coins_minor: 100,
          stock: 0,
          enabled: false,
          content: x.content,
        })),
      };
    else if (p === "/api/v1/exchange/rewards/items") {
      const b = r.request().postDataJSON();
      writes.push(b);
      data = { saved: true };
    }
    return r.fulfill({ json: { data } });
  });
  return writes;
}
test("physical reward library composes a pack, retries and supplies exchange listing", async ({
  page,
}, info) => {
  const writes = await setup(page);
  await page.goto("/rewards");
  await page.getByRole("button", { name: "新增奖励", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog
    .getByRole("textbox", { name: "奖励名称", exact: true })
    .fill("入门礼包");
  await dialog
    .getByRole("combobox", { name: "添加配装舰船", exact: true })
    .click();
  await page.getByRole("option", { name: "训练护卫", exact: true }).click();
  await dialog.getByRole("button", { name: "添加", exact: true }).click();
  await expect(
    dialog.getByRole("combobox", { name: "添加配装舰船" }),
  ).toHaveCount(0);
  await dialog.getByRole("spinbutton", { name: "训练护卫数量" }).fill("2");
  await dialog.getByRole("button", { name: "移除训练护卫" }).click();
  await dialog.getByRole("combobox", { name: "添加配装舰船" }).click();
  await page.getByRole("option", { name: "训练护卫", exact: true }).click();
  await dialog.getByRole("button", { name: "添加", exact: true }).click();
  await dialog.getByRole("textbox", { name: "搜索奖励物品" }).fill("三钛");
  await dialog.getByRole("button", { name: "三钛合金", exact: true }).click();
  await dialog.getByRole("spinbutton", { name: "三钛合金数量" }).fill("100");
  await dialog.getByRole("spinbutton", { name: "ISK 奖励" }).fill("123.45");
  await expect(
    dialog.getByRole("spinbutton", { name: "果壳币奖励" }),
  ).toHaveCount(0);
  await dialog.getByRole("button", { name: "保存", exact: true }).click();
  await expect(dialog.getByRole("alert")).toHaveText("保存失败");
  await expect(dialog.getByRole("textbox", { name: "奖励名称" })).toHaveValue(
    "入门礼包",
  );
  mkdirSync("../.local/catalog", { recursive: true });
  await page.screenshot({
    path: `../.local/catalog/editor-${info.project.name}.png`,
  });
  await dialog.getByRole("button", { name: "保存", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect(writes[0].request_key).toBe(writes[1].request_key);
  expect(writes[1].content).toEqual({
    isk_minor: 12345,
    fittings: [{ fitting_id: "1", quantity: 1 }],
    items: [{ type_id: "34", quantity: 100 }],
  });
  await page.getByRole("button", { name: "兑换设置", exact: true }).click();
  const form = page.getByRole("form", { name: "奖励物品配置" });
  await form.getByRole("combobox", { name: "上架状态" }).click();
  await page.getByRole("option", { name: "已上架", exact: true }).click();
  await expect(form).toContainText("训练护卫");
  await expect(form).toContainText("三钛合金");
  await form
    .getByRole("spinbutton", { name: "可用库存（份）", exact: true })
    .fill("3");
  await form.getByRole("combobox", { name: "定价方式" }).click();
  await page
    .getByRole("option", { name: "自动核价 · 每 6 小时", exact: true })
    .click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "保存", exact: true })
    .click();
  expect(writes[2]).toMatchObject({
    id: "5",
    stock: 3,
    enabled: true,
    automatic_pricing: true,
  });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
});

stateLayouts({
  name: "rewards",
  setup: setup,
  path: "/rewards",
  endpoint: "**/api/v1/exchange/catalog?**",
  empty: { items: [], next_cursor: "" },
  emptyText: /暂无奖励|No rewards/,
});

test("long reward names and short viewport editor retain reachable actions", async ({
  page,
}, info) => {
  await setup(page);
  const name = "LongRewardName".repeat(12);
  await page.route("**/api/v1/exchange/catalog?**", (r) =>
    r.fulfill({
      json: {
        data: {
          items: [
            {
              id: "7",
              version: "1",
              name,
              archived: false,
              content: {
                fittings: [],
                items: [{ type_id: "34", quantity: 100 }],
              },
            },
          ],
          next_cursor: "",
        },
      },
    }),
  );
  await page.goto("/rewards");
  await expect(
    page.getByRole("button", { name: "编辑 " + name, exact: true }),
  ).toBeVisible();
  for (const width of [1440, 375, 320]) {
    await page.setViewportSize({ width, height: 568 });
    await assertLayout(page);
  }
  await page.getByRole("button", { name: "编辑 " + name, exact: true }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  await assertLayout(page);
  await expect(
    dialog.getByRole("button", { name: "保存", exact: true }),
  ).toBeInViewport();
  await expect(
    dialog.getByRole("button", { name: "关闭", exact: true }),
  ).toBeInViewport();
  await page.screenshot({ path: info.outputPath("short-dialog-320.png") });
  await dialog.getByRole("textbox", { name: "奖励名称", exact: true }).focus();
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "编辑 " + name, exact: true }),
  ).toBeFocused();
});
