import { test, expect, type Page } from "@playwright/test";
import { mkdirSync } from "node:fs";

async function setup(
  page: Page,
  {
    allowed = true,
    administrator = allowed,
    partial = false,
    exchange = true,
    metrics = true,
    empty = false,
    language = "zh-CN",
  } = {},
) {
  const reads: string[] = [];
  await page.addInitScript(
    (l) => localStorage.setItem("glorynavy.locale", l),
    language,
  );
  await page.route("https://images.evetech.net/**", (r) =>
    r.fulfill({
      contentType: "image/svg+xml",
      body: '<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64"><rect width="64" height="64" fill="#e2e8f0"/><circle cx="32" cy="25" r="13" fill="#94a3b8"/></svg>',
    }),
  );
  await page.route("**/api/v1/**", (r) => {
    const u = new URL(r.request().url()),
      p = u.pathname;
    reads.push(p + u.search);
    let data: unknown = {};
    if (p.endsWith("/modules"))
      data = [
        "system",
        "identity",
        "eve",
        "access",
        "approval",
        "welfare",
        ...(metrics ? ["attendance", "wallet"] : []),
        "skills",
        "fittings",
        ...(exchange ? ["exchange"] : []),
      ].map((id) => ({ id, version: "0.1.0", api_version: 1 }));
    else if (p.endsWith("/session"))
      data = {
        authenticated: true,
        session: {
          user_id: "account-a",
          character: { id: "123", name: "Login Alt" },
          main_character: { id: "124", name: "Main Pilot" },
          csrf_token: "csrf",
          expires_at: "2099-01-01T00:00:00Z",
        },
      };
    else if (p.endsWith("/access/me"))
      data = {
        administrator,
        can_manage: allowed,
        can_manage_sync: false,
        characters: [],
        site_roles: [],
      };
    else if (p.endsWith("/identity/characters"))
      data = {
        characters: empty
          ? []
          : Array.from({ length: 5 }, (_, i) => ({
              id: String(124 + i),
              name: i ? `Pilot ${i} LongCharacterName` : "Main Pilot",
              is_main: i === 0,
              status: i === 4 ? "blocked" : "active",
            })),
      };
    else if (p.endsWith("/attendance/pap-requirement"))
      data = {
        source: "alliance",
        monthly_points: 3,
        version: "1",
        can_manage: administrator,
      };
    else if (p.endsWith("/attendance/alliance-pap"))
      data = {
        source: "alliance",
        month: "2026-09",
        points: empty ? 0 : 3,
        available: !empty,
        complete: !empty,
        state: empty ? "syncing" : "ready",
        records_total: 72,
        last_synced_at: "2026-09-21T17:26:39Z",
        version: "1",
        can_manage: administrator,
        characters: empty
          ? []
          : [
              { character_id: 124, character_name: "Main Pilot", pap: 2 },
              {
                character_id: 125,
                character_name: "Pilot 1 LongCharacterName",
                pap: 1,
              },
            ],
      };
    else if (p.endsWith("/attendance/context"))
      data = {
        corporations: [],
        characters: [{ id: "124", name: "Main Pilot" }],
      };
    else if (p.endsWith("/attendance/pap"))
      data = {
        points: empty ? 0 : 150,
        events: empty ? 0 : 12,
        participations: empty ? 0 : 25,
        rows: [],
        more: true,
      };
    else if (p.endsWith("/wallet/context"))
      data = {
        owners: empty
          ? []
          : [
              {
                kind: "corporation",
                id: "10",
                name: "Corporation wallet",
                divisions: [1],
                journal: true,
                transactions: true,
              },
              {
                kind: "character",
                id: "125",
                name: "Alt Pilot",
                divisions: [0],
                journal: true,
                transactions: true,
              },
              {
                kind: "character",
                id: "124",
                name: "Main Pilot",
                divisions: [0],
                journal: true,
                transactions: true,
              },
            ],
      };
    else if (p.endsWith("/wallet/summary"))
      data = {
        items: empty ? [] : [
          { owner_id: "125", balance: "0", observed_at: "2026-09-21T03:00:00Z", income: "0", expense: "1000000000" },
          { owner_id: "124", balance: "1234567890.12", observed_at: "2026-09-21T03:00:00Z", income: "3000000000", expense: "0" },
        ],
      };
    else if (p.endsWith("/wallet/records"))
      data = {
        items: [
          {
            id: "1",
            division: 0,
            observed_at: "2026-09-21T03:00:00Z",
            ...(u.searchParams.get("part") === "journal"
              ? {
                  amount:
                    u.searchParams.get("owner_id") === "124"
                      ? "3000000000"
                      : "-1000000000",
                }
              : {
                  balance:
                    u.searchParams.get("owner_id") === "124"
                      ? "1234567890.12"
                      : "0",
                }),
          },
        ],
        names: {},
        next_cursor: "",
      };
    else if (p.endsWith("/approval/context"))
      data = {
        allowed,
        sources: allowed ? ["welfare", "exchange"] : [],
        corporations: [],
        people: [],
        unavailable: [],
      };
    else if (p.endsWith("/approval/items"))
      data = {
        items: empty
          ? []
          : Array.from({ length: 7 }, (_, i) => ({
              source: "welfare",
              id: String(i + 1),
              version: "1",
              account_id: "someone-else",
              corporation_id: "10",
              applicant: "Member Main",
              kind: "solo",
              state: "submitted",
              status: "",
              recipient: "Recipient",
              title:
                u.searchParams.get("view") === "exceptions"
                  ? "Contract mismatch"
                  : "PVP补损",
              reference: "WF-test",
              amount_minor: 1000,
              unit: "isk",
              time: "2026-09-21T03:00:00Z",
              action: "",
              actions: ["approve"],
              payload: {},
            })),
        counts: {
          pending: empty ? 0 : 27,
          fulfillment: 8,
          exceptions: 2,
          information: 1,
          history: 100,
        },
        next_cursor: "next",
        unavailable: partial ? ["welfare"] : [],
      };
    else if (p.endsWith("/exchange/rewards"))
      data = {
        admin: allowed,
        isk_per_coin: 10000,
        version: "1",
        earned_minor: 1250550,
        reserved_minor: 10000,
        spent_minor: 40500,
        available_minor: 1200050,
        rewards: [],
        next_cursor: "",
      };
    else if (p.endsWith("/exchange/rewards/orders"))
      data = {
        items: empty
          ? []
          : [
              {
                id: "7",
                version: "1",
                type_id: "34",
                name: "巡洋舰配装奖励 / Cruiser fitting package",
                quantity: 1,
                recipient_id: "124",
                recipient_name: "Main Pilot",
                coins_minor: 10000,
                isk_per_coin: 10000,
                isk_value: 1000000,
                state: "pending",
                note: "",
                created_at: "2026-09-21T03:00:00Z",
              },
            ],
        next_cursor: "",
      };
    return r.fulfill({ json: { data } });
  });
  return reads;
}

test("工作台按授权显示审批，使用完整统计并跳转对应单据", async ({ page }) => {
  await setup(page);
  await page.goto("/workspace");
  await expect(page.getByRole("button", { name: "待审批 27" })).toBeVisible();
  await expect(page.locator(".desk-queue li")).toHaveCount(5);
  await expect(page.locator(".desk-identity")).toHaveText("Main Pilot");
  await expect(page.locator(".desk-balance strong").first()).toHaveText(
    "12,000.5",
  );
  await expect(page.locator(".desk-queue a").first()).toHaveAttribute(
    "href",
    "/approvals?source=welfare&id=1",
  );
  await page.getByRole("button", { name: "异常 2" }).click();
  await expect(page.locator(".desk-queue")).toContainText("Contract mismatch");
  await expect(page.locator(".desk-section-heading a")).toHaveAttribute(
    "href",
    "/approvals?view=exceptions",
  );
});

test("管理员待办无需等待审批上下文返回", async ({ page }) => {
  const reads = await setup(page);
  let releaseContext!: () => void;
  const contextGate = new Promise<void>((resolve) => {
    releaseContext = resolve;
  });
  await page.route("**/api/v1/approval/context", async (route) => {
    await contextGate;
    await route.fulfill({
      json: {
        data: {
          allowed: true,
          sources: ["welfare", "exchange"],
          corporations: [],
          people: [],
          unavailable: [],
        },
      },
    });
  });
  try {
    await page.goto("/workspace");
    await expect.poll(() => reads.some((p) => p.startsWith("/api/v1/approval/items?view=pending"))).toBe(true);
    expect(reads.indexOf("/api/v1/access/me")).toBeLessThan(
      reads.findIndex((p) => p.startsWith("/api/v1/approval/items?view=pending")),
    );
  } finally {
    releaseContext();
  }
  await expect(page.getByRole("button", { name: "待审批 27" })).toBeVisible();
});

test("工作台预读审批分类，切换复用缓存", async ({ page }) => {
  const reads = await setup(page);
  await page.goto("/workspace");
  await expect(page.getByRole("button", { name: "待审批 27" })).toBeVisible();
  await expect.poll(() =>
    ["fulfillment", "exceptions", "information"].every((view) =>
      reads.some((path) => path === `/api/v1/approval/items?view=${view}`),
    ),
  ).toBe(true);
  const before = reads.filter((path) => path === "/api/v1/approval/items?view=exceptions").length;
  await page.getByRole("button", { name: "异常 2" }).click();
  await expect(page.locator(".desk-queue")).toContainText("Contract mismatch");
  expect(reads.filter((path) => path === "/api/v1/approval/items?view=exceptions")).toHaveLength(before);
});

test("审批分类首次加载时保留已知计数", async ({ page }) => {
  await setup(page);
  let release!: () => void;
  const gate = new Promise<void>((resolve) => { release = resolve; });
  await page.route("**/api/v1/approval/items?view=fulfillment", async (route) => {
    await gate;
    await route.fallback();
  });
  try {
    await page.goto("/workspace");
    await expect(page.getByRole("button", { name: "待审批 27" })).toBeVisible();
    await page.getByRole("button", { name: "待发放 8" }).click();
    await expect(page.getByRole("button", { name: "待审批 27" })).toBeVisible();
    await expect(page.getByRole("button", { name: "待发放 8" })).toBeVisible();
    await expect(page.locator(".desk-queue")).toContainText("正在读取");
  } finally {
    release();
  }
  await expect(page.locator(".desk-queue li")).toHaveCount(5);
});

test("普通成员不读取审批队列，不加载关闭模块", async ({ page }) => {
  const reads = await setup(page, {
    allowed: false,
    exchange: false,
    metrics: false,
  });
  await page.goto("/workspace");
  await expect(page.locator(".desk-characters a")).toHaveCount(5);
  await expect(page.locator(".desk-review")).toHaveCount(0);
  expect(reads.some((p) => p.startsWith("/api/v1/approval/items"))).toBe(false);
  expect(reads.some((p) => p.startsWith("/api/v1/exchange/"))).toBe(false);
  expect(reads.some((p) => p.startsWith("/api/v1/attendance/"))).toBe(false);
  expect(reads.some((p) => p.startsWith("/api/v1/wallet/"))).toBe(false);
});

test("本人联盟PAP与全部角色钱包，同排卡片等高", async ({ page }) => {
  const reads = await setup(page);
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/workspace");
  const pap = page
    .locator(".desk-panel")
    .filter({ has: page.getByRole("heading", { name: "联盟 PAP" }) });
  await expect(
    pap.getByRole("img", { name: "本月联盟 PAP: 3 / 3 PAP" }),
  ).toBeVisible();
  await expect(pap).toContainText("达标");
  await expect(pap).not.toContainText("72");
  await expect(page.locator(".desk-isk strong")).toHaveText("1,234,567,890.12");
  await expect(page.locator(".desk-wallet-list li")).toHaveCount(3);
  await expect(page.locator(".desk-wallet-list li").first()).toContainText(
    "Main Pilot",
  );
  await expect(page.getByText("已读取 2/5 个角色钱包")).toBeVisible();
  const sizes = await page
    .locator(".desk-personal > .desk-panel")
    .evaluateAll((nodes) =>
      nodes.map((n) => ({
        top: n.getBoundingClientRect().top,
        height: n.getBoundingClientRect().height,
      })),
    );
  expect(sizes[0]).toEqual(sizes[1]);
  expect(sizes[2]).toEqual(sizes[3]);
  await expect(page.locator(".desk-wallet-list li").nth(1)).toContainText(
    "0.00",
  );
  await expect(page.locator(".desk-wallet-list")).not.toContainText(
    "Corporation wallet",
  );
  expect(new Set(reads.filter((p) => p.startsWith("/api/v1/wallet/summary?"))).size).toBe(1);
  expect(reads.some((p) => p.startsWith("/api/v1/wallet/records"))).toBe(false);
  expect(reads.some((p) => p.startsWith("/api/v1/wallet/context"))).toBe(false);
  const requests = reads
    .filter((p) => p.startsWith("/api/v1/attendance/alliance-pap"))
    .map((p) => new URL(p, "http://local"));
  expect(requests.length).toBeGreaterThan(0);
  await expect(page.locator(".desk-wallet-speed")).toContainText("赚钱速度");
  await expect(pap.locator(".desk-more")).toHaveAttribute(
    "href",
    "/attendance?view=alliance-pap",
  );
});

test("钱包无快照与PAP失败不冒充零值", async ({ page }) => {
  await setup(page);
  await page.route("**/api/v1/attendance/alliance-pap", (r) =>
    r.fulfill({ status: 503, json: { error: { message: "Unavailable" } } }),
  );
  await page.route("**/api/v1/wallet/summary?**", (r) =>
    r.fulfill({ json: { data: { items: [
      { owner_id: "124", balance: null, observed_at: null, income: "0", expense: "0" },
      { owner_id: "125", balance: null, observed_at: null, income: "0", expense: "0" },
    ] } } }),
  );
  await page.goto("/workspace");
  const pap = page
    .locator(".desk-panel")
    .filter({ has: page.getByRole("heading", { name: "联盟 PAP" }) });
  await expect(pap.getByRole("alert")).toContainText("数据暂不可用");
  await expect(pap.locator(".desk-balance")).toHaveCount(0);
  await expect(page.locator(".desk-isk strong")).toHaveText("—");
  await expect(
    page.getByText("暂无余额记录", { exact: true }).first(),
  ).toBeVisible();
});

test("全部钱包精确合计，缺少单角色快照只显示已读取余额", async ({ page }) => {
  await setup(page);
  await page.route("**/api/v1/identity/characters", (r) =>
    r.fulfill({
      json: {
        data: {
          characters: [
            { id: "124", name: "Main Pilot", is_main: true, status: "active" },
            { id: "125", name: "Alt Pilot", is_main: false, status: "active" },
          ],
        },
      },
    }),
  );
  await page.route("**/api/v1/wallet/summary?**", (r) =>
    r.fulfill({ json: { data: { items: [
      { owner_id: "124", balance: "0.1", observed_at: "2026-09-21T03:00:00Z", income: "1", expense: "0" },
      { owner_id: "125", balance: "0.2", observed_at: "2026-09-21T03:00:00Z", income: "0", expense: "1" },
    ] } } }),
  );
  await page.goto("/workspace");
  await expect(page.locator(".desk-isk")).toContainText("全部角色合计（ISK）");
  await expect(page.locator(".desk-isk strong")).toHaveText("0.30");
  await page.route("**/api/v1/wallet/summary?**", (r) =>
    r.fulfill({ json: { data: { items: [
      { owner_id: "124", balance: "0.1", observed_at: "2026-09-21T03:00:00Z", income: "1", expense: "0" },
      { owner_id: "125", balance: null, observed_at: null, income: "0", expense: "0" },
    ] } } }),
  );
  await page.reload();
  await expect(page.locator(".desk-isk")).toContainText(
    "已读取余额合计（ISK）",
  );
  await expect(page.locator(".desk-isk strong")).toHaveText("0.10");
});

test("部分失败不显示错误零值，空状态独立展示", async ({ page }) => {
  await setup(page, { partial: true, empty: true });
  await page.goto("/workspace");
  await expect(page.getByRole("alert")).toContainText("部分待办暂不可用");
  await expect(page.locator(".desk-metrics strong")).toHaveText([
    "—",
    "—",
    "—",
    "—",
  ]);
  await expect(page.locator(".desk-queue")).toContainText("暂无可显示的待办");
  await expect(page.getByText("暂无兑换记录")).toBeVisible();
});

test("有业务审批权限的非站点管理员也不显示工作台审批待办", async ({ page }) => {
  const reads = await setup(page, { allowed: true, administrator: false });
  await page.goto("/workspace");
  await expect(page.locator(".desk-characters a")).toHaveCount(5);
  await expect(page.locator(".desk-review")).toHaveCount(0);
  expect(reads.some((p) => p.startsWith("/api/v1/approval/items"))).toBe(false);
});

test("联盟PAP要求配置与军团PAP统计分开", async ({ page }, info) => {
  await setup(page);
  let points = 3,
    version = 1;
  await page.route("**/api/v1/attendance/pap-requirement", async (r) => {
    if (r.request().method() === "POST") {
      expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
      const b = r.request().postDataJSON();
      expect(b.version).toBe(String(version));
      points = b.monthly_points;
      version++;
      return r.fulfill({ json: { data: { saved: true } } });
    }
    return r.fulfill({
      json: {
        data: {
          source: "alliance",
          monthly_points: points,
          version: String(version),
          can_manage: true,
        },
      },
    });
  });
  await page.goto("/attendance?view=pap");
  await expect(
    page.getByRole("heading", { name: "军团 PAP 明细", exact: true }),
  ).toBeVisible();
  await expect(page.locator(".pap-requirement")).toHaveCount(0);
  await page.getByRole("tab", { name: "联盟 PAP", exact: true }).click();
  await expect(page.getByRole("img", { name: /本月联盟 PAP/ })).toHaveCount(1);
  await page
    .getByRole("button", { name: "集结分要求配置", exact: true })
    .click();
  await page
    .getByRole("spinbutton", { name: "每人每月最低联盟 PAP" })
    .fill("151");
  await page.getByRole("button", { name: "保存", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(page.locator(".pap-requirement")).toContainText("联盟 PAP");
  await page.getByRole("tab", { name: "军团 PAP", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "军团 PAP 明细", exact: true }),
  ).toBeVisible();
  mkdirSync("../docs/ui/reviews/workspace", { recursive: true });
  await page.screenshot({
    path: `../docs/ui/reviews/workspace/${info.project.name}-attendance-pap.png`,
    fullPage: true,
  });
  await page.goto("/workspace");
  await expect(
    page.locator(".desk-panel").filter({ hasText: "联盟 PAP" }),
  ).toBeVisible();
});

test("联盟PAP要求不可用时不伪造军团PAP达标图", async ({ page }) => {
  await setup(page);
  let points = 0;
  await page.route("**/api/v1/attendance/pap?**", (r) =>
    r.fulfill({
      json: {
        data: {
          points,
          events: 1,
          participations: 1,
          rows: [],
          more: false,
        },
      },
    }),
  );
  await page.route("**/api/v1/attendance/pap-requirement", (r) =>
    r.fulfill({ status: 503, json: { error: { message: "Unavailable" } } }),
  );
  await page.goto("/workspace");
  await expect(page.getByText("集结分要求暂不可用")).toBeVisible();
  await expect(page.getByRole("progressbar")).toHaveCount(0);
  await expect(page.locator(".desk-pap-number")).toHaveText("3");
  await expect(page.locator(".desk-pap-overview [role=img]")).toHaveCount(0);
});

for (const language of ["zh-CN", "en"])
  test(`工作台响应式 ${language}`, async ({ page }, info) => {
    await setup(page, { language });
    await page.goto("/workspace");
    await expect(page.locator(".desk-queue li")).toHaveCount(5);
    await expect(page.getByRole("progressbar")).toHaveCount(0);
    for (const width of [1440, 768, 375, 320]) {
      await page.setViewportSize({ width, height: 1000 });
      await expect
        .poll(() =>
          page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth,
          ),
        )
        .toBe(true);
      const buttons = page.locator(".desk-metrics button");
      expect(
        await buttons
          .first()
          .evaluate((el) => el.getBoundingClientRect().height),
      ).toBeGreaterThanOrEqual(44);
      mkdirSync("../docs/ui/reviews/workspace", { recursive: true });
      await page.screenshot({
        path: `../docs/ui/reviews/workspace/${info.project.name}-${language}-${width}.png`,
        fullPage: true,
      });
    }
  });

test("联盟图表遵循配置门槛，完整钱包显示真实收支速度", async ({ page }) => {
  await setup(page, { administrator: false });
  await page.route("**/api/v1/attendance/pap-requirement", (r) =>
    r.fulfill({
      json: {
        data: {
          source: "alliance",
          monthly_points: 5,
          version: "2",
          can_manage: false,
        },
      },
    }),
  );
  await page.route("**/api/v1/identity/characters", (r) =>
    r.fulfill({
      json: {
        data: {
          characters: [
            { id: "124", name: "Main Pilot", is_main: true, status: "active" },
            { id: "125", name: "Alt Pilot", is_main: false, status: "active" },
          ],
        },
      },
    }),
  );
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/workspace");
  await expect(
    page.getByRole("img", { name: "本月联盟 PAP: 3 / 5 PAP" }),
  ).toBeVisible();
  await expect(page.getByText("还差 2 PAP")).toBeVisible();
  await expect(
    page.locator(".desk-wallet-speed strong").first(),
  ).not.toHaveText("—");
  const amounts = await page
    .locator(".desk-wallet-speed strong")
    .allTextContents();
  expect(
    Number(amounts[0].replaceAll(",", "")) /
      Number(amounts[1].replaceAll(",", "")),
  ).toBeCloseTo(3, 5);
  await page.screenshot({
    path: "../docs/ui/reviews/workspace/desktop-wallet-complete.png",
    fullPage: true,
  });
});
