import { stateLayouts } from "./state-layout";
import { test, expect, type Page } from "@playwright/test";
import { englishLayout } from "./language-layout";
import { mkdirSync } from "node:fs";
const now = new Date().toISOString();
const entry = {
  id: "1",
  corporation_id: "900",
  name: "集结近程护卫",
  description: "近程火力",
  version: "1",
  updated_at: now,
  ship_name: "裂谷级",
  group: "护卫舰",
  eft: "[Rifter, 集结近程护卫]\nDamage Control II",
  fit: {
    name: "集结近程护卫",
    ship_type_id: "587",
    skill_mode: "all5",
    items: [
      { type_id: "2048", slot: "low", index: 0, quantity: 1, state: "active" },
    ],
  },
};
async function setup(
  page: Page,
  admin = true,
  canSave = true,
  unknown = false,
) {
  const writes: {
    path: string;
    body: Record<string, unknown>;
    csrf: string | undefined;
  }[] = [];
  await page.route("https://images.evetech.net/**", (r) => r.abort());
  await page.route("**/api/v1/**", async (r) => {
    const p = new URL(r.request().url()).pathname,
      method = r.request().method();
    let data: unknown;
    if (method !== "GET")
      writes.push({
        path: p,
        body: r.request().postDataJSON(),
        csrf: r.request().headers()["x-csrf-token"],
      });
    if (p.endsWith("/modules"))
      data = ["system", "identity", "eve", "access", "fittings", "skills"].map(
        (id) => ({ id, version: "0.1.0", api_version: 1 }),
      );
    else if (p.endsWith("/identity/session"))
      data = {
        authenticated: true,
        session: {
          user_id: "user",
          character: { id: "123", name: "Hajimi1" },
          csrf_token: "csrf",
          expires_at: "2099-01-01T00:00:00Z",
        },
      };
    else if (p.endsWith("/access/me"))
      data = {
        administrator: admin,
        can_manage: admin,
        can_manage_sync: false,
        site_roles: [],
        characters: [],
      };
    else if (p.endsWith("/library/context"))
      data = {
        can_manage: admin,
        corporations: [
          { id: "900", name: "Glory Navy", can_create_skills: admin },
        ],
        characters: [{ id: "123", name: "Hajimi1", can_save: canSave }],
      };
    else if (p.endsWith("/fittings/context"))
      data = { characters: [{ id: "123", name: "Hajimi1" }], can_edit: true };
    else if (p.endsWith("/fittings/names"))
      data = [
        { id: "2048", name: "损伤控制 II" },
        { id: "587", name: "裂谷级" },
      ];
    else if (p.endsWith("/saved"))
      data = {
        status: "ready",
        observed_at: now,
        skills: {},
        fittings: [
          {
            id: "9",
            name: "个人护卫",
            description: "",
            ship_type_id: "587",
            items: [{ type_id: "2048", flag: "LoSlot0", quantity: 1 }],
          },
        ],
      };
    else if (p.endsWith("/requirements"))
      data = {
        name: entry.name,
        corporation_id: "900",
        build: "3503375",
        requirements: [{ skill_id: "3300", level: 3 }],
      };
    else if (p.endsWith("/save-to-game"))
      data = {
        id: "7",
        state: unknown ? "unknown" : "saved",
        fitting_id: unknown ? null : "99",
        reason: unknown ? "confirmation_required" : "",
      };
    else if (p.endsWith("/library"))
      data =
        method === "GET"
          ? [entry]
          : {
              ...entry,
              id: "2",
              name: "新方案",
              fit: { ...entry.fit, name: "新方案" },
            };
    else if (p.endsWith("/library/1")) data = entry;
    else if (p.endsWith("/skills/catalog"))
      data = {
        build: "3503375",
        items: [
          {
            id: "3300",
            name: "射击学",
            english: "Gunnery",
            group: "射击学",
            group_id: "255",
          },
          {
            id: "3301",
            name: "小型混合炮台",
            english: "Small Hybrid Turret",
            group: "射击学",
            group_id: "255",
          },
        ],
      };
    else if (p.includes("/skills/plans"))
      data = {
        ...r.request().postDataJSON(),
        id: "12",
        version: "1",
        updated_at: now,
      };
    else data = {};
    await r.fulfill({ json: { data } });
  });
  await page.goto("/fittings");
  await expect(
    page.getByRole("heading", { name: "舰船配置", exact: true }),
  ).toBeVisible();
  return writes;
}
async function detail(page: Page) {
  await expect(page.locator(".library-choices button").first()).toBeAttached();
  if (await page.locator(".library-choices button").first().isVisible())
    await page.locator(".library-choices button").first().click();
  await expect(page.getByRole("heading", { name: entry.name })).toBeVisible();
}
test("English fittings layout", async ({ page }, info) => {
  await setup(page);
  await englishLayout(page, "Fittings", `fittings-${info.project.name}`);
  await expect(
    page.getByRole("button", { name: "Import fitting", exact: true }),
  ).toBeVisible();
});

test("方案库筛选、个人方案与响应式，无模拟资源", async ({ page }, info) => {
  const assets: string[] = [];
  page.on("request", (r) => {
    if (/\.wasm|\.dat/.test(r.url())) assets.push(r.url());
  });
  await setup(page);
  await detail(page);
  await expect(page.getByText("损伤控制 II", { exact: true })).toBeVisible();
  await expect(page.getByText("CPU", { exact: true })).toHaveCount(0);
  mkdirSync(".local/library-review", { recursive: true });
  await page.screenshot({
    path: `.local/library-review/${info.project.name}.png`,
    fullPage: true,
  });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBeTruthy();
  await page.setViewportSize({ width: 320, height: 900 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBeTruthy();
  await page.screenshot({
    path: `.local/library-review/${info.project.name}-320.png`,
    fullPage: true,
  });
  await page.getByRole("button", { name: "返回方案列表" }).click();
  await page.getByRole("textbox", { name: "搜索方案或船型" }).fill("不存在");
  await expect(page.getByText("没有匹配方案")).toBeVisible();
  await page.getByRole("button", { name: "个人方案", exact: true }).click();
  await page.getByRole("textbox", { name: "搜索方案或船型" }).fill("");
  await page.locator(".library-choices button").first().click();
  await expect(page.getByRole("heading", { name: "个人护卫" })).toBeVisible();
  expect(assets).toEqual([]);
});
test("管理员导入、替换、删除；写入携带版本和CSRF", async ({ page }) => {
  const writes = await setup(page);
  await page.getByRole("button", { name: "导入配置", exact: true }).click();
  await page
    .getByLabel("装配方案", { exact: true })
    .fill("[Rifter, 新方案]\nDamage Control II");
  await page.getByRole("button", { name: "导入军团库" }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(writes[0].body.eft).toContain("新方案");
  expect(writes[0].csrf).toBe("csrf");
  await detail(page);
  await page.getByRole("button", { name: "替换配置" }).click();
  await page.getByRole("button", { name: "保存替换" }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(writes[1].body.version).toBe("1");
  await page.getByRole("button", { name: "删除配置" }).click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "删除配置" })
    .click();
  await expect.poll(() => writes.length).toBe(3);
});
test("成员保存到游戏，成功禁用重复提交", async ({ page }) => {
  const writes = await setup(page, false);
  await detail(page);
  await expect(page.getByRole("button", { name: "导入配置" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "替换配置" })).toHaveCount(0);
  await page.getByRole("button", { name: "保存到个人配置" }).click();
  await page.getByRole("button", { name: "保存到游戏", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("已保存到 Hajimi1");
  await expect(
    page.getByRole("button", { name: "保存到游戏", exact: true }),
  ).toBeDisabled();
  expect(writes).toHaveLength(1);
  expect(writes[0].body.character_id).toBe("123");
  expect(writes[0].csrf).toBe("csrf");
});
test("缺授权不能写；不确定结果禁止重发", async ({ page }) => {
  const writes = await setup(page, false, false);
  await detail(page);
  await page.getByRole("button", { name: "保存到个人配置" }).click();
  await expect(page.getByRole("link", { name: "前往重新授权" })).toBeVisible();
  await expect(
    page.getByRole("button", { name: "保存到游戏", exact: true }),
  ).toBeDisabled();
  expect(writes).toHaveLength(0);
  await page.unrouteAll();
  await setup(page, false, true, true);
  await detail(page);
  await page.getByRole("button", { name: "保存到个人配置" }).click();
  await page.getByRole("button", { name: "保存到游戏", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("游戏中核对");
  await expect(
    page.getByRole("button", { name: "保存到游戏", exact: true }),
  ).toBeDisabled();
});
test("配置生成技能方案，可改等级并快速再编辑", async ({ page }) => {
  const writes = await setup(page);
  await detail(page);
  await page.getByRole("button", { name: "创建技能方案" }).click();
  await expect(page.getByLabel("方案名称")).toHaveValue(entry.name);
  await page.getByRole("combobox", { name: "射击学 要求等级" }).click();
  await page.getByRole("option", { name: "5 级", exact: true }).click();
  await page.getByLabel("添加技能").fill("小型混合");
  await page.getByRole("button", { name: "小型混合炮台" }).click();
  await page.getByRole("button", { name: "保存要求" }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(writes[0].body.requirements).toEqual([
    { skill_id: "3300", level: 5 },
    { skill_id: "3301", level: 5 },
  ]);
  await page.getByRole("button", { name: "编辑技能方案" }).click();
  await expect(
    page.getByRole("heading", { name: "编辑技能要求" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "移除小型混合炮台" }).click();
  await page.getByRole("button", { name: "保存要求" }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(writes[1].path).toContain("/plans/12");
});

stateLayouts({ name: "fittings", setup: setup, path: "/fittings", endpoint: "**/api/v1/fittings/library?**", empty: [], emptyText: /暂无配置方案|No fittings/ });
