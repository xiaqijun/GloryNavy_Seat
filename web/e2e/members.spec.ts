import { stateLayouts } from "./state-layout";
import { openNavigation } from "./navigation";
import { test, expect, type Page } from "@playwright/test";
import { mkdirSync } from "node:fs";
const actor = "00000000-0000-4000-8000-000000000001",
  member = "00000000-0000-4000-8000-000000000002",
  second = "00000000-0000-4000-8000-000000000003";
const fact = (id: string) => ({
  character_id: id,
  state: "ready",
  corporation: {
    id: "789",
    name: "Glory Navy",
    alliance_id: "0",
    ceo_id: "999",
  },
  roles: ["Accountant", "Factory_Manager"],
  roles_at_hq: [],
  roles_at_base: [],
  roles_at_other: [],
  synced_at: "2026-09-14T00:00:00Z",
  valid_until: "2099-01-01T00:00:00Z",
});
const chars = [
  { id: "202", name: "远航后勤主角色", is_main: true, status: "active" },
  {
    id: "203",
    name: "工业分队第二角色长名称",
    is_main: false,
    status: "active",
  },
];
const memberAccess = {
  administrator: false,
  site_roles: [{ id: "role-1", name: "后勤组" }],
  characters: chars.map((c) => fact(c.id)),
};
async function setup(page: Page, initialAdmin = true, community = true) {
  let admin = initialAdmin;
  const calls: string[] = [];
  await page.route("https://images.evetech.net/**", (r) => r.abort());
  await page.route("**/api/v1/**", (r) => {
    const url = new URL(r.request().url()),
      path = url.pathname;
    calls.push(path + url.search);
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
          user_id: actor,
          character: { id: "101", name: "管理员角色" },
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
    if (path === "/api/v1/access/members")
      return json({
        items:
          url.searchParams.get("q") === "不存在"
            ? []
            : [
                {
                  user_id: url.searchParams.get("after") ? second : member,
                  character_id: "202",
                  name: url.searchParams.get("after")
                    ? "下一页成员"
                    : "远航后勤主角色",
                  character_count: 2,
                  administrator: false,
                  roles: [],
                },
              ],
        next: url.searchParams.get("after") ? "" : member,
      });
    if (path.endsWith("/data")) {
      if (!admin)
        return r.fulfill({
          status: 403,
          json: { error: { message: "仅站点管理员可查看成员数据" } },
        });
      if (path.includes(second))
        return r.fulfill({
          status: 404,
          json: { error: { message: "成员不存在或已不可用" } },
        });
      return json({
        user_id: member,
        characters: chars,
        access: memberAccess,
        community: community
          ? {
              qq: { value: "123456789", confirmation: "pending" },
              kook: {
                value: "荣耀远航后勤指挥频道昵称",
                confirmation: "confirmed",
              },
            }
          : null,
      });
    }
    if (path.includes("/sync/characters/"))
      return json({
        available: true,
        targets: [
          {
            id: "1",
            character_id: path.split("/").at(-1),
            name: "角色",
            resource: "character_contracts",
            state: "idle",
            reason: "",
            freshness: "fresh",
            last_attempt_at: null,
            last_success_at: "2026-09-14T00:00:00Z",
            content_updated_at: null,
            next_due_at: "2099-01-01T00:00:00Z",
          },
        ],
      });
    if (path === "/api/v1/eve/contracts/owners")
      return json({
        owners: chars.map((c) => ({
          kind: "character",
          id: c.id,
          name: c.name,
        })),
      });
    if (path.startsWith("/api/v1/eve/contracts/character/"))
      return json({ items: [], next_cursor: "" });
    return r.fulfill({
      status: 404,
      json: { error: { message: "未知测试接口" } },
    });
  });
  return {
    calls,
    demote: () => {
      admin = false;
    },
  };
}
test("administrator member directory, pagination and character contract navigation", async ({
  page,
}) => {
  const { calls } = await setup(page);
  await page.goto("/members");
  await openNavigation(page, "系统管理");
  await expect(
    page.getByRole("link", { name: "成员", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "下一页成员" }).click();
  await expect(page.getByText("下一页成员", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "上一页成员" }).click();
  await expect(page.getByText("第 1 页")).toBeVisible();
  expect(new URL(page.url()).searchParams.get("after")).toBeNull();
  await page.getByRole("button", { name: /远航后勤主角色/ }).click();
  await expect(
    page.getByRole("heading", { name: "远航后勤主角色", exact: true }),
  ).toBeVisible();
  await expect(page.getByText("123456789", { exact: true })).toBeVisible();
  await expect(page.getByText("会计", { exact: true })).toBeVisible();
  await page
    .getByRole("button", { name: "工业分队第二角色长名称", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "工业分队第二角色长名称", exact: true }),
  ).toBeVisible();
  await page.getByRole("link", { name: "查看合同" }).click();
  await expect(page.getByRole("combobox", { name: "查看角色" })).toContainText("工业分队第二角色长名称");
  expect(
    calls.some((c) => c === `/api/v1/eve/contracts/owners?member=${member}`),
  ).toBeTruthy();
  await page.getByRole("link", { name: "返回成员资料" }).click();
  await expect(page.getByText("123456789", { exact: true })).toBeVisible();
  await expect(
    page.getByRole("button", {
      name: /解绑|添加角色|重新授权|保存|设为主角色/,
    }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "返回成员列表" }).click();
  await page.getByRole("textbox", { name: "搜索成员" }).fill("不存在");
  await page.getByRole("button", { name: "搜索成员", exact: true }).click();
  await expect(page.getByText("没有匹配的成员")).toBeVisible();
});
test("management-only users cannot enter member data or see navigation", async ({
  page,
}) => {
  const { calls } = await setup(page, false);
  await page.goto(`/members?member=${member}`);
  await expect(page.getByText("仅站点管理员可查看成员数据")).toBeVisible();
  await expect(
    page.getByRole("link", { name: "成员", exact: true }),
  ).toHaveCount(0);
  expect(calls.some((c) => c.endsWith("/data"))).toBeFalsy();
});
test("demotion and unavailable members clear previously displayed private data", async ({
  page,
}) => {
  const controller = await setup(page);
  await page.goto(`/members?member=${member}`);
  await expect(page.getByText("123456789")).toBeVisible();
  controller.demote();
  await page.getByRole("button", { name: "刷新成员资料" }).click();
  await expect(page.getByRole("alert")).toContainText("仅站点管理员");
  await expect(page.getByText("123456789")).toHaveCount(0);
  await setup(page);
  await page.goto(`/members?member=${second}`);
  await expect(page.getByRole("alert")).toContainText("成员不存在");
  await expect(page.getByText("123456789")).toHaveCount(0);
});
test("member data tolerates disabled community module", async ({ page }) => {
  await setup(page, true, false);
  await page.goto(`/members?member=${member}`);
  await expect(page.getByText("社区模块未启用")).toBeVisible();
  await expect(page.getByRole("link", { name: "查看合同" })).toBeVisible();
});
test("member layout and keyboard at desktop and narrow widths", async ({
  page,
}, info) => {
  await setup(page);
  mkdirSync("../docs/ui/reviews/members", { recursive: true });
  for (const width of [1440, 375, 320]) {
    await page.setViewportSize({ width, height: 1000 });
    await page.goto(`/members?member=${member}`);
    await expect(
      page.getByRole("heading", { name: "远航后勤主角色", exact: true }),
    ).toBeVisible();
    const alt = page.getByRole("button", {
      name: "工业分队第二角色长名称",
      exact: true,
    });
    await alt.focus();
    await page.keyboard.press("Enter");
    await expect(alt).toHaveAttribute("aria-pressed", "true");
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBeTruthy();
    await page.screenshot({
      path: `../docs/ui/reviews/members/detail-${info.project.name}-${width}.png`,
      fullPage: true,
    });
  }
});

stateLayouts({ name: "members", setup: setup, path: "/members", endpoint: "**/api/v1/access/members?**", empty: { items: [], next: "" }, emptyText: /没有匹配的成员|No matching members/ });
