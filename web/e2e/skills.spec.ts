import { stateLayouts } from "./state-layout";
import { navigate } from "./navigation";
import { test, expect, type Page } from "@playwright/test";
import { mkdirSync } from "node:fs";

test("legitimate nested overlays do not report an orphan interaction lock", async ({
  page,
}) => {
  await setup(page);
  await page.getByRole("button", { name: "军团要求", exact: true }).click();
  await page.getByRole("button", { name: "编辑技能要求" }).click();
  await page.getByRole("dialog").getByRole("combobox").first().click();
  await page.waitForTimeout(3500);
  expect(
    await page.evaluate(() =>
      sessionStorage.getItem("gnv:interaction-diagnostics"),
    ),
  ).toBeNull();
  await page.keyboard.press("Escape");
  await page.keyboard.press("Escape");
  await navigate(page, "工作台");
  expect(
    await page
      .locator("body")
      .evaluate((el) => getComputedStyle(el).pointerEvents),
  ).toBe("auto");
});
const now = new Date().toISOString(),
  future = new Date(Date.now() + 3600000).toISOString(),
  past = new Date(Date.now() - 3600000).toISOString();
async function setup(page: Page, manage = true, blocked = false) {
  await page.route("https://images.evetech.net/**", (r) => r.abort());
  await page.route("**/api/v1/modules", (r) =>
    r.fulfill({
      json: {
        data: ["system", "identity", "eve", "access", "skills"].map((id) => ({
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
            user_id: "user",
            character: { id: "123", name: "Hajimi1" },
            csrf_token: "csrf",
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
          can_manage: false,
          can_manage_sync: false,
          site_roles: [],
          characters: [],
        },
      },
    }),
  );
  const skill = {
    id: "3300",
    name: "射击学",
    english: "Gunnery",
    group: "射击学",
    group_id: "255",
  };
  let plans = [
    {
      id: "1",
      corporation_id: "900",
      name: "集结基础",
      requirements: [{ skill_id: "3300", level: 5 }],
      version: "1",
      updated_at: now,
    },
  ];
  await page.route("**/api/v1/skills/**", async (r) => {
    const u = new URL(r.request().url()),
      p = u.pathname;
    let data: unknown;
    if (p.endsWith("/context"))
      data = {
        characters: [{ id: "123", name: "Hajimi1", corporation_id: "900" }],
        corporations: [{ id: "900", name: "Glory Navy", can_manage: manage }],
      };
    else if (p.endsWith("/catalog"))
      data = {
        build: "3503375",
        items: [
          skill,
          {
            ...skill,
            id: "3301",
            name: "小型混合炮台",
            english: "Small Hybrid Turret",
          },
        ],
      };
    else if (p.includes("/characters/")) {
      const meta = {
        status: blocked ? "blocked" : "ready",
        reason: blocked ? "missing_scope" : "",
        observed_at: blocked ? null : now,
        valid_until: future,
      };
      data = {
        skills_meta: meta,
        queue_meta: meta,
        skills: blocked
          ? []
          : [
              {
                ...skill,
                trained: 5,
                active: 4,
                points: 256000,
                queue_applied: false,
              },
              {
                ...skill,
                id: "3301",
                name: "小型混合炮台",
                trained: 3,
                active: 3,
                points: 8000,
                queue_applied: false,
              },
            ],
        queue: blocked
          ? []
          : [
              {
                id: "3301",
                name: "小型混合炮台",
                level: 4,
                position: 0,
                start: past,
                finish: future,
                state: "training",
              },
              {
                id: "3300",
                name: "射击学",
                level: 5,
                position: 1,
                start: null,
                finish: null,
                state: "paused",
              },
            ],
        total_sp: blocked ? null : 264000,
        unallocated_sp: blocked ? null : 5000,
        calculated_at: now,
      };
    } else if (p.endsWith("/check"))
      data = [
        {
          character: { id: "123", name: "Hajimi1", corporation_id: "900" },
          state: blocked ? "unknown" : "met",
          met: blocked ? 0 : 1,
          total: 1,
          remaining_sp: blocked ? null : 0,
          observed_at: blocked ? null : now,
          checks: [
            {
              skill_id: "3300",
              name: "射击学",
              required: 5,
              remaining_sp: blocked ? null : 0,
              trained: blocked ? null : 5,
              state: blocked ? "unknown" : "met",
            },
          ],
        },
      ];
    else if (r.request().method() !== "GET") {
      expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
      const b = r.request().postDataJSON();
      expect(b).not.toHaveProperty("updated_at");
      expect(b).not.toHaveProperty("id");
      const isCreate = r.request().method() === "POST";
      data = {
        id: isCreate ? "2" : p.split("/").at(-1),
        ...b,
        version: "2",
        updated_at: now,
      };
      if (r.request().method() === "DELETE")
        plans = plans.filter((v) => v.id !== p.split("/").at(-1));
      else if (isCreate) plans.push(data as (typeof plans)[number]);
      else
        plans = plans.map((v) =>
          v.id === (data as { id: string }).id ? (data as typeof v) : v,
        );
    } else data = plans;
    if (p.endsWith("/check")) data = { items: data, next_cursor: "" };
    // Fixture models the server's localized display fields, preserving plan titles.
    if (r.request().headers()["accept-language"] === "en") {
      data = JSON.parse(JSON.stringify(data), (key, value) => {
        if (key !== "name" && key !== "group") return value;
        return value === "射击学"
          ? "Gunnery"
          : value === "小型混合炮台"
            ? "Small Hybrid Turret"
            : value;
      });
    }
    await r.fulfill({ json: { data } });
  });
  await page.goto("/skills");
  await expect(
    page.getByRole("heading", { name: "技能管理", exact: true }),
  ).toBeVisible();
}
test("English skills and editable requirement dialog preserve original plan names", async ({
  page,
}, info) => {
  await setup(page);
  await page.getByRole("button", { name: "切换为英文", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Skills", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Corporation requirements", exact: true })
    .click();
  await expect(page.getByText("集结基础", { exact: true })).toBeVisible();
  await page
    .getByRole("button", { name: "Edit skill requirements", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(
    dialog.getByRole("button", { name: "Save requirements", exact: true }),
  ).toBeVisible();
  await expect(dialog.getByLabel("Plan name", { exact: true })).toHaveValue(
    "集结基础",
  );
  await page.setViewportSize({
    width: info.project.name === "mobile" ? 375 : 1280,
    height: 900,
  });
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          document.documentElement.scrollWidth <=
          document.documentElement.clientWidth,
      ),
    )
    .toBe(true);
  await page.screenshot({
    path: `../.local/i18n-skills-${info.project.name}.png`,
    fullPage: true,
  });
  await dialog.getByRole("button", { name: "Close", exact: true }).click();
});

test("skills overview, queue and responsive layouts", async ({
  page,
}, info) => {
  await setup(page);
  await expect(page.getByText("当前生效 4 级")).toBeVisible();
  await page.getByRole("textbox", { name: "搜索技能" }).fill("混合");
  await expect(page.locator(".skill-row")).toHaveCount(1);
  await page.getByRole("textbox", { name: "搜索技能" }).fill("");
  const desktop = info.project.name === "desktop";
  for (const width of desktop ? [1440, 375, 320] : [375, 320]) {
    await page.setViewportSize({ width, height: 1000 });
    await expect(page.locator(".skill-row")).toHaveCount(2);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBeTruthy();
    mkdirSync(".local/skills/screenshots", { recursive: true });
    await page.screenshot({
      path: `.local/skills/screenshots/${info.project.name}-${width}.png`,
      fullPage: true,
    });
  }
  await page.getByRole("button", { name: "训练队列", exact: true }).click();
  await expect(page.getByText("训练中", { exact: true })).toBeVisible();
  await expect(page.getByText("已暂停", { exact: true })).toBeVisible();
  await expect(page.getByRole("progressbar")).toBeVisible();
});

test("background refresh keeps skills and queue visible until replacement", async ({
  page,
}) => {
  await setup(page);
  let snapshot = {
    skills_meta: {
      status: "stale",
      reason: "",
      observed_at: past,
      valid_until: null as string | null,
    },
    queue_meta: {
      status: "stale",
      reason: "",
      observed_at: past,
      valid_until: null as string | null,
    },
    skills: [
      {
        id: "3300",
        name: "射击学",
        group: "射击学",
        trained: 4,
        active: 4,
        points: 45255,
        queue_applied: false,
      },
    ],
    queue: [
      {
        id: "3300",
        name: "射击学",
        level: 5,
        position: 0,
        start: past,
        finish: future,
        state: "training",
      },
    ],
    total_sp: 45255,
    unallocated_sp: 5000,
    calculated_at: now,
  };
  let release: (() => void) | undefined;
  let waitForSync: Promise<void> | undefined;
  await page.route("**/api/v1/skills/characters/123", async (route) => {
    if (waitForSync) await waitForSync;
    await route.fulfill({ json: { data: snapshot } });
  });
  await page.reload();
  await expect(page.locator(".skill-row")).toHaveCount(1);
  await expect(page.locator(".skill-row")).toContainText("45,255 SP");
  await expect(page.getByText(/待更新|待同步|等待同步/)).toHaveCount(0);
  waitForSync = new Promise<void>((resolve) => {
    release = resolve;
  });
  try {
    await page.getByRole("button", { name: "刷新技能数据" }).click();
    await expect(
      page.getByRole("button", { name: "刷新技能数据" }),
    ).toBeDisabled();
    await expect(page.locator(".skill-row")).toContainText("45,255 SP");
    await page.getByRole("button", { name: "训练队列", exact: true }).click();
    await expect(page.getByText("训练中", { exact: true })).toBeVisible();
    await expect(page.getByText(/待更新|待同步|等待同步/)).toHaveCount(0);
    snapshot = {
      ...snapshot,
      skills_meta: {
        status: "ready",
        reason: "",
        observed_at: now,
        valid_until: future,
      },
      queue_meta: {
        status: "ready",
        reason: "",
        observed_at: now,
        valid_until: future,
      },
      skills: [
        { ...snapshot.skills[0], trained: 5, active: 5, points: 256000 },
      ],
      total_sp: 256000,
      queue: [],
    };
  } finally {
    release?.();
  }
  await expect(page.getByText("暂无待训练技能")).toBeVisible();
  await page.getByRole("button", { name: "技能总览", exact: true }).click();
  await expect(page.locator(".skill-row")).toContainText("256,000 SP");
  await expect(page.getByText(/待更新|待同步|等待同步/)).toHaveCount(0);
});
test("skill check shows total and individual SP gaps with responsive unknown and zero states", async ({
  page,
}, info) => {
  await setup(page);
  await page.route("**/api/v1/skills/plans/1/check?*", (r) =>
    r.fulfill({
      json: {
        data: {
          items: [
            {
              character: { id: "123", name: "Hajimi1", corporation_id: "900" },
              state: "missing",
              met: 1,
              total: 3,
              remaining_sp: 25505,
              observed_at: past,
              checks: [
                {
                  skill_id: "3302",
                  name: "已满足的技能不重复展示",
                  required: 2,
                  trained: 5,
                  state: "met",
                  remaining_sp: 0,
                },
                {
                  skill_id: "3300",
                  name: "射击学",
                  required: 4,
                  trained: 3,
                  state: "missing",
                  remaining_sp: 25255,
                },
                {
                  skill_id: "3301",
                  name: "小型混合炮台",
                  required: 1,
                  trained: 0,
                  state: "missing",
                  remaining_sp: 250,
                },
              ],
            },
            {
              character: {
                id: "124",
                name: "已完成的角色",
                corporation_id: "900",
              },
              state: "met",
              met: 1,
              total: 1,
              remaining_sp: 0,
              observed_at: now,
              checks: [
                {
                  skill_id: "3300",
                  name: "射击学",
                  required: 4,
                  trained: 5,
                  state: "met",
                  remaining_sp: 0,
                },
              ],
            },
            {
              character: {
                id: "125",
                name: "尚无记录的角色",
                corporation_id: "900",
              },
              state: "unknown",
              met: 0,
              total: 1,
              remaining_sp: null,
              observed_at: null,
              checks: [
                {
                  skill_id: "3300",
                  name: "射击学",
                  required: 4,
                  trained: null,
                  state: "unknown",
                  remaining_sp: null,
                },
              ],
            },
          ],
          next_cursor: "",
        },
      },
    }),
  );
  await page.getByRole("button", { name: "军团要求", exact: true }).click();
  const rows = page.locator(".skill-results .skill-result-card");
  await expect(rows.first().locator("summary")).toContainText("还差 25,505 SP");
  await rows.first().locator("summary").click();
  await expect(rows.first().locator(".skill-checks")).toContainText(
    "还差 25,255 SP",
  );
  await expect(rows.first().locator(".skill-checks .skills-row")).toHaveCount(2);
  await expect(rows.first()).not.toContainText("已满足的技能不重复展示");
  await expect(rows.nth(1).locator(".skill-result-heading")).toContainText("已达标");
  await expect(rows.nth(1).locator("summary, .skill-checks")).toHaveCount(0);
  await expect(rows.nth(1).locator(".skill-sp-gap")).toHaveCount(0);
  await expect(rows.nth(2).locator("summary")).toContainText("剩余 SP —");
  for (const width of info.project.name.includes("desktop")
    ? [1440, 375, 320]
    : [375, 320]) {
    await page.setViewportSize({ width, height: 900 });
    await rows.first().scrollIntoViewIfNeeded();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBeTruthy();
    const columns = await rows.first().locator(".skill-checks").evaluate((el) => getComputedStyle(el).gridTemplateColumns.split(" ").length);
    expect(columns).toBe(width === 1440 ? 2 : 1);
    await expect(rows.first().locator("summary")).toContainText(
      "还差 25,505 SP",
    );
    mkdirSync(".local/skills/screenshots", { recursive: true });
    await page.screenshot({
      path: `.local/skills/screenshots/sp-gap-${info.project.name}-${width}.png`,
    });
  }
  await page.getByRole("button", { name: "刷新技能数据" }).click();
  await expect(rows.first().locator("summary")).toContainText("还差 25,505 SP");
});

test("corporation plans create edit check delete", async ({ page }) => {
  await setup(page);
  await page.getByRole("button", { name: "军团要求", exact: true }).click();
  await expect(page.getByRole("heading", { name: "达标检查" })).toBeVisible();
  await expect(page.locator(".skill-result-heading")).toContainText("已达标");
  await expect(page.locator(".skill-results summary")).toHaveCount(0);
  await expect(page.locator(".skill-plan-requirements")).not.toHaveAttribute("open", "");
  await page.locator(".skill-plan-requirements summary").click();
  await expect(page.locator(".skill-requirements")).toBeVisible();
  await page.getByRole("button", { name: "新建要求" }).click();
  await page.getByRole("textbox", { name: "方案名称" }).fill("炮术要求");
  await page.getByRole("textbox", { name: "添加技能" }).fill("混合");
  await page.getByRole("button", { name: "小型混合炮台 射击学" }).click();
  await page.getByRole("button", { name: "保存要求" }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(
    page.getByRole("combobox", { name: "技能要求方案" }),
  ).toContainText("炮术要求");
  await page.getByRole("button", { name: "编辑技能要求" }).click();
  await page.getByRole("textbox", { name: "方案名称" }).fill("炮术基础");
  await page.getByRole("button", { name: "保存要求" }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(
    page.getByRole("combobox", { name: "技能要求方案" }),
  ).toContainText("炮术基础");
  await page.getByRole("button", { name: "删除技能要求" }).click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "删除要求" })
    .click();
  await expect(
    page.getByRole("combobox", { name: "技能要求方案" }),
  ).toContainText("集结基础");
});
test("ordinary member cannot manage and missing authorization is unknown", async ({
  page,
}) => {
  await setup(page, false, true);
  await expect(page.getByText("请在我的角色页面重新授权")).toBeVisible();
  await page.getByRole("button", { name: "军团要求", exact: true }).click();
  await expect(page.getByText("待确认", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "新建要求" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "编辑技能要求" })).toHaveCount(
    0,
  );
  await expect(page.getByRole("combobox", { name: "检查范围" })).toHaveCount(0);
});

test("shared skill dialogs keep actions visible and restore keyboard focus", async ({
  page,
}, info) => {
  await setup(page);
  await page.route("**/api/v1/skills/plans?*", (r) =>
    r.fulfill({
      json: {
        data: [
          {
            id: "1",
            corporation_id: "900",
            name: "长技能方案",
            version: "1",
            updated_at: now,
            requirements: Array.from({ length: 40 }, (_, i) => ({
              skill_id: String(3300 + i),
              level: 5,
            })),
          },
        ],
      },
    }),
  );
  await page.getByRole("button", { name: "军团要求", exact: true }).click();
  const opener = page.getByRole("button", { name: "编辑技能要求" });
  await opener.click();
  const dialog = page.getByRole("dialog", { name: "编辑技能要求" });
  for (const width of info.project.name === "desktop"
    ? [1440, 375, 320]
    : [375, 320]) {
    await page.setViewportSize({ width, height: 720 });
    await expect(dialog).toBeVisible();
    for (const item of [
      dialog.getByRole("heading"),
      dialog.getByRole("button", { name: "保存要求" }),
    ]) {
      const box = await item.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.y).toBeGreaterThanOrEqual(0);
      expect(box!.y + box!.height).toBeLessThanOrEqual(720);
    }
    expect(
      await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth),
    ).toBeTruthy();
    const levelBox = await dialog.getByRole("combobox").first().boundingBox();
    const removeBox = await dialog
      .getByRole("button", { name: "移除射击学", exact: true })
      .boundingBox();
    expect(Math.abs(levelBox!.y - removeBox!.y)).toBeLessThan(1);
    await page.screenshot({
      path: `.local/skills/screenshots/dialog-${info.project.name}-${width}.png`,
    });
  }
  await dialog.getByRole("combobox").first().click();
  await expect(
    page.getByRole("option", { name: "4 级", exact: true }),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(dialog).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(opener).toBeFocused();
  await page.getByRole("button", { name: "删除技能要求" }).click();
  const confirm = page.getByRole("alertdialog", { name: "删除技能要求" });
  await expect(confirm.getByRole("button", { name: "取消" })).toBeFocused();
  await page.screenshot({
    path: `.local/skills/screenshots/confirm-${info.project.name}.png`,
  });
  await page.keyboard.press("Escape");
  await expect(confirm).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "删除技能要求" }),
  ).toBeFocused();
});

test("delete confirmation keeps errors and blocks dismissal while saving", async ({
  page,
}) => {
  await setup(page);
  await page.getByRole("button", { name: "军团要求", exact: true }).click();
  let release: (() => void) | undefined;
  const pending = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/v1/skills/plans/1", async (r) => {
    if (r.request().method() !== "DELETE") return r.fallback();
    await pending;
    await r.fulfill({
      status: 409,
      json: { error: { message: "方案已更新，请重试" } },
    });
  });
  await page.getByRole("button", { name: "删除技能要求" }).click();
  const confirm = page.getByRole("alertdialog");
  try {
    await confirm.getByRole("button", { name: "删除要求" }).click();
    await expect(confirm.getByRole("button", { name: "取消" })).toBeDisabled();
    await page.keyboard.press("Escape");
    await expect(confirm).toBeVisible();
  } finally {
    release?.();
  }
  await expect(confirm.getByRole("alert")).toHaveText("方案已更新，请重试");
  await expect(confirm.getByRole("button", { name: "删除要求" })).toBeEnabled();
  await confirm.getByRole("button", { name: "取消" }).click();
  await expect(
    page.getByRole("combobox", { name: "技能要求方案" }),
  ).toContainText("集结基础");
});

stateLayouts({ name: "skills", setup: setup, path: "/skills", endpoint: "**/api/v1/skills/context**", empty: { characters: [], corporations: [] }, emptyText: /暂无可用角色|No available characters/ });
