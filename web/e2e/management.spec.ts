import { stateLayouts } from "./state-layout";
import { openNavigation } from "./navigation";
import { test, expect, type Page } from "@playwright/test";
import type { Role } from "../src/modules/access/manage-api";

const user = "01994763-4111-7000-8000-111111111111";
const member = "01994763-4111-7000-8000-222222222222";
async function setup(page: Page, allowed = true, initialRoles: Role[] = []) {
  let roles: Role[] = initialRoles;
  let assigned: string[] = [];
  let conflict = false;
  let writes = 0;
  await page.route("https://images.evetech.net/**", (r) => r.abort());
  await page.route("**/api/v1/modules", (r) =>
    r.fulfill({
      json: {
        data: ["system", "identity", "eve", "access"].map((id) => ({
          id,
          version: "0.1.0",
          api_version: 1,
        })),
      },
    }),
  );
  await page.route("**/api/v1/identity/session", (r) =>
    r.fulfill({
      json: {
        data: {
          authenticated: true,
          session: {
            user_id: user,
            character: { id: "123", name: "管理舰长" },
            csrf_token: "test-csrf",
            expires_at: "2099-01-01T00:00:00Z",
          },
        },
      },
    }),
  );
  await page.route("**/api/v1/access/me", (r) =>
    r.fulfill({
      json: {
        data: {
          administrator: false,
          can_manage: allowed,
          site_roles: [],
          characters: [],
        },
      },
    }),
  );
  await page.route("**/api/v1/access/catalog", (r) =>
    r.fulfill({
      json: {
        data: [{ id: "access.manage", label: "权限管理", scope: "global" }],
      },
    }),
  );
  await page.route("**/api/v1/access/roles", (r) =>
    r.fulfill({ json: { data: roles } }),
  );
  await page.route("**/api/v1/access/roles/*", (r) => {
    expect(r.request().headers()["x-csrf-token"]).toBe("test-csrf");
    writes++;
    const url = new URL(r.request().url()),
      id = url.pathname.split("/").at(-1)!;
    if (r.request().method() === "DELETE") {
      expect(url.searchParams.get("version")).toBe(
        roles.find((x) => x.id === id)?.version,
      );
      roles = roles.filter((x) => x.id !== id);
      assigned = assigned.filter((x) => x !== id);
      return r.fulfill({ json: { data: { saved: true } } });
    }
    const body = r.request().postDataJSON();
    if (conflict) {
      conflict = false;
      roles = roles.map((x) =>
        x.id === id
          ? {
              ...x,
              name: "其他管理员的修改",
              version: String(Number(x.version) + 1),
            }
          : x,
      );
      return r.fulfill({
        status: 409,
        json: { error: { message: "角色已变更，请读取最新内容后重试" } },
      });
    }
    const previous = roles.find((x) => x.id === id);
    expect(body.version).toBe(previous?.version ?? "0");
    roles = [
      ...roles.filter((x) => x.id !== id),
      { ...body, id, version: String(Number(body.version) + 1) },
    ];
    return r.fulfill({ json: { data: { saved: true } } });
  });
  await page.route("**/api/v1/access/members?*", (r) =>
    r.fulfill({
      json: {
        data: {
          items: [
            {
              user_id: member,
              character_id: "456",
              name: "远航工业与后勤支援舰长",
              character_count: 5,
              administrator: false,
              roles: roles.filter((x) => assigned.includes(x.id)),
            },
          ],
          next: "",
        },
      },
    }),
  );
  await page.route("**/api/v1/access/users/*/roles/*", (r) => {
    expect(r.request().headers()["x-csrf-token"]).toBe("test-csrf");
    expect(r.request().url()).toContain(member);
    const id = r.request().url().split("/").at(-1)!;
    assigned = r.request().method() === "PUT" ? [id] : [];
    return r.fulfill({ json: { data: { saved: true } } });
  });
  await page.route("**/api/v1/access/audit?*", (r) =>
    r.fulfill({
      json: {
        data: {
          items: [
            {
              id: "1",
              actor: user,
              subject: member,
              action: "role.assigned",
              created_at: "2026-09-14T10:00:00Z",
            },
          ],
          next: "",
        },
      },
    }),
  );
  return {
    get roles() {
      return roles;
    },
    get writes() {
      return writes;
    },
    conflict: () => {
      conflict = true;
    },
  };
}

test("shipped permissions, assignment, concurrency recovery and deletion", async ({
  page,
}, info) => {
  const state = await setup(page);
  await page.goto("/access");
  await openNavigation(page, "系统管理");
  await expect(
    page.getByRole("navigation").getByRole("link", { name: "权限管理" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "新建角色" }).click();
  await expect(page.getByText("军团业务", { exact: true })).toHaveCount(0);
  await expect(page.getByText("钱包与资产分部", { exact: true })).toHaveCount(
    0,
  );
  await expect(page.getByRole("checkbox")).toHaveCount(1);
  await page.getByRole("button", { name: "保存角色", exact: true }).click();
  expect(state.writes).toBe(0);
  await expect(page.getByRole("alert")).toContainText("请输入 1–80 字");
  await page.getByLabel("角色名称", { exact: true }).fill("角色管理员");
  await page.getByRole("checkbox", { name: "权限管理", exact: true }).check();
  await page.getByRole("button", { name: "保存角色", exact: true }).click();
  await expect(page.getByRole("status")).toHaveText("角色已保存");
  expect(state.roles[0].grants).toEqual([
    { permission: "access.manage", corporations: [], alliances: [] },
  ]);
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({
    path: `../.local/permissions-roles-${info.project.name}.png`,
    fullPage: true,
  });
  if (info.project.name === "desktop") {
    for (const width of [375, 320]) {
      await page.setViewportSize({ width, height: 850 });
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
      ).toBe(true);
      await page.evaluate(() => window.scrollTo(0, 0));
      await page.screenshot({
        path: `../.local/permissions-roles-${width}.png`,
        fullPage: true,
      });
    }
    await page.setViewportSize({ width: 1280, height: 900 });
  }
  state.conflict();
  await page.getByLabel("角色名称", { exact: true }).fill("角色修改草稿");
  await page.getByRole("button", { name: "保存角色", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("角色已变更");
  await expect(page.getByLabel("角色名称", { exact: true })).toHaveValue(
    "角色修改草稿",
  );
  await page.getByRole("button", { name: "读取最新内容" }).click();
  await expect(page.getByLabel("角色名称", { exact: true })).toHaveValue(
    "其他管理员的修改",
  );
  await page.getByRole("tab", { name: "成员授权" }).click();
  await page.getByRole("button", { name: /远航工业与后勤支援舰长/ }).click();
  const portrait = await page
    .locator(".access-member-row .eve-image")
    .boundingBox();
  expect(portrait?.width).toBe(portrait?.height);
  expect(portrait?.width).toBeLessThanOrEqual(64);
  await page
    .getByLabel("授予角色", { exact: true })
    .selectOption(state.roles[0].id);
  await page.getByRole("button", { name: "授予", exact: true }).click();
  await expect(page.getByRole("status")).toHaveText("角色已授予");
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({
    path: `../.local/permissions-members-${info.project.name}.png`,
    fullPage: true,
  });
  await page.getByRole("button", { name: "撤销其他管理员的修改" }).click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "确认", exact: true })
    .click();
  await expect(page.getByRole("status")).toHaveText("角色已撤销");
  await page.getByRole("tab", { name: "操作记录" }).click();
  await page.getByText("授予角色", { exact: true }).click();
  await expect(page.getByText(user, { exact: true })).toBeVisible();
  await page.getByRole("tab", { name: "角色配置" }).click();
  await page.getByRole("button", { name: "删除其他管理员的修改" }).click();
  await expect(page.getByRole("alertdialog")).toContainText("全部成员授权");
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "确认", exact: true })
    .click();
  await expect(
    page.getByRole("group", { name: "平台角色列表" }).getByRole("button"),
  ).toHaveCount(0);
});

test("ordinary member cannot discover menu or load management data", async ({
  page,
}) => {
  await setup(page, false);
  let restricted = 0;
  page.on("request", (r) => {
    if (/access\/(catalog|roles|members|audit)/.test(r.url())) restricted++;
  });
  await page.goto("/access");
  await expect(
    page.getByText("没有权限管理权限", { exact: true }),
  ).toBeVisible();
  await openNavigation(page, "系统管理");
  await expect(
    page.getByRole("navigation").getByRole("link", { name: "权限管理" }),
  ).toHaveCount(0);
  expect(restricted).toBe(0);
});

test("editing published permissions preserves hidden historical grants", async ({
  page,
}) => {
  const legacy: Role = {
    id: member,
    name: "旧角色",
    version: "1",
    grants: [
      {
        permission: "corporation.journal",
        corporations: ["98530802"],
        alliances: [],
      },
    ],
  };
  const state = await setup(page, true, [legacy]);
  await page.goto("/access");
  await page
    .getByRole("group", { name: "平台角色列表" })
    .getByRole("button", { name: /旧角色 0 项权限/ })
    .click();
  await expect(page.getByRole("checkbox")).toHaveCount(1);
  await expect(page.getByLabel("军团 ID", { exact: true })).toHaveCount(0);
  await page.getByLabel("角色名称", { exact: true }).fill("保留历史授权");
  await page.getByRole("button", { name: "保存角色", exact: true }).click();
  await expect(page.getByRole("status")).toHaveText("角色已保存");
  expect(state.roles[0].grants).toEqual(legacy.grants);
});

stateLayouts({ name: "access", setup: setup, path: "/access", endpoint: "**/api/v1/access/roles" });
