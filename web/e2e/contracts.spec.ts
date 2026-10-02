import { stateLayouts } from "./state-layout";
import { test, expect, type Page } from "@playwright/test";
import { mkdirSync } from "node:fs";
const entity = (id: string, name: string, category = "character") => ({
  id,
  name,
  category,
});
const sample = {
  id: "9001",
  title: "舰队补给 · 巡洋舰与弹药",
  type: "item_exchange",
  status: "outstanding",
  availability: "public",
  for_corporation: false,
  issuer: entity("123", "荣耀远航后勤舰长"),
  assignee: entity("0", ""),
  acceptor: entity("0", ""),
  start: entity(
    "60003760",
    "Jita IV - Moon 4 - Caldari Navy Assembly Plant",
    "station",
  ),
  end: entity("0", ""),
  price: "123456789012345.67",
  reward: null,
  collateral: null,
  buyout: null,
  volume: "4250",
  days_to_complete: null,
  date_issued: "2026-09-14T00:00:00Z",
  date_expired: "2026-09-20T00:00:00Z",
  date_accepted: "",
  date_completed: "",
  checked_at: "2026-09-14T04:00:00Z",
};

test("ESI status is not inferred from expiry or a completion prefix", async ({ page }) => {
  for (const status of ["outstanding", "finished_issuer", "finished_contractor", "finished_future", "finished"]) {
    await setup(page, { status, date_expired: "2020-01-01T00:00:00Z" });
    await page.goto("/contracts");
    const badge = page.locator(`.contract-status[title="${status}"]`).first();
    await expect(badge).toBeVisible();
    if (status === "finished") await expect(badge).toHaveClass(/is-done/);
    else await expect(badge).not.toHaveClass(/is-done/);
    if (status === "outstanding") await expect(badge).toHaveText("未决");
    if (status === "finished_future") await expect(badge).toHaveText(status);
    if (status === "finished_issuer" || status === "finished_contractor")
      await expect(badge).toHaveClass(/is-active/);
  }
});
async function setup(
  page: Page,
  overrides: Partial<typeof sample> & {
    summary?: string;
    trade_direction?: "sell" | "buy" | "exchange" | "transport" | "unknown";
  } = {},
) {
  const current = { ...sample, ...overrides };
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
            user_id: "contracts-test",
            character: { id: "123", name: "远航舰长" },
            csrf_token: "test",
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
  await page.route("**/api/v1/eve/contracts/**", (r) => {
    const url = new URL(r.request().url()),
      path = url.pathname;
    if (path.endsWith("/owners"))
      return r.fulfill({
        json: {
          data: {
            owners: [
              { kind: "character", id: "123", name: "远航舰长" },
              { kind: "character", id: "456", name: "工业分队舰长" },
              { kind: "corporation", id: "789", name: "Glory Navy" },
            ],
          },
        },
      });
    if (path.includes("/999"))
      return r.fulfill({
        status: 404,
        json: { error: { message: "合同不存在或没有查看权限" } },
      });
    if (path.endsWith("/items"))
      return r.fulfill({
        json: {
          data: {
            items: [
              {
                id: "1",
                type: entity("34", "三钛合金", "inventory_type"),
                quantity: "150000",
                included: true,
                singleton: false,
                raw_quantity: null,
              },
              {
                id: "2",
                type: entity("12005", "伊什塔级蓝图", "inventory_type"),
                quantity: "1",
                included: false,
                singleton: true,
                raw_quantity: -2,
              },
            ],
            next_cursor: "",
          },
        },
      });
    if (path.endsWith("/bids"))
      return r.fulfill({
        json: {
          data: {
            items: [
              {
                id: "1",
                bidder: entity("456", "工业分队舰长"),
                amount: "123456789012345.67",
                date: "2026-09-14T00:00:00Z",
              },
            ],
            next_cursor: "",
          },
        },
      });
    if (/\/900[12]$/.test(path))
      return r.fulfill({
        json: {
          data: {
            contract: {
              ...current,
              ...(path.endsWith("9002")
                ? {
                    id: "9002",
                    title: "舰队旗舰拍卖",
                    type: "auction",
                    buyout: "200000000000000",
                  }
                : {}),
            },
            details: [
              {
                part: "items",
                state: "ready",
                reason: "",
                updated_at: current.checked_at,
              },
              {
                part: "bids",
                state: "pending",
                reason: "rate_limited",
                updated_at: null,
              },
            ],
          },
        },
      });
    const empty = url.searchParams.get("q") === "找不到";
    const items = empty
      ? []
      : url.searchParams.get("before")
        ? [{ ...current, id: "9002", title: "舰队旗舰拍卖", type: "auction" }]
        : [
            current,
            {
              ...current,
              id: "8999",
              title: "后勤运输委托",
              type: "courier",
              trade_direction: "transport",
              reward: "75000000",
              price: null,
              status: "in_progress",
            },
            {
              ...current,
              id: "8998",
              title: "远征舰队装备交接",
              assignee: entity("456", "工业分队舰长"),
              acceptor: entity("888", "联盟接收专员"),
              status: "finished",
              price: "1500000000",
            },
          ];
    return r.fulfill({
      json: {
        data: {
          items,
          next_cursor: empty || url.searchParams.get("before") ? "" : "9001",
        },
      },
    });
  });
}
test("explicit owner loads in parallel but waits for owner confirmation before rendering", async ({ page }) => {
  await setup(page);
  let release!: () => void;
  const gate = new Promise<void>((resolve) => { release = resolve; });
  await page.route("**/api/v1/eve/contracts/owners", async (route) => {
    await gate;
    await route.fulfill({ json: { data: { owners: [] } } });
  });
  const list = page.waitForResponse((r) => new URL(r.url()).pathname === "/api/v1/eve/contracts/character/123");
  try {
    await page.goto("/contracts?kind=character&owner=123");
    await list;
    await expect(page.getByRole("table", { name: "合同列表" })).toHaveCount(0);
    release();
    await expect(page.getByText("该合同范围不可用", { exact: true })).toBeVisible();
    await expect(page.getByRole("table", { name: "合同列表" })).toHaveCount(0);
  } finally { release(); }
});

test("anonymous contract deep links never prefetch private contract data", async ({ page }) => {
  await setup(page);
  await page.route("**/api/v1/identity/session", (route) => route.fulfill({ json: { data: { authenticated: false, session: null } } }));
  const requests: string[] = [];
  page.on("request", (r) => { if (r.url().includes("/api/v1/eve/contracts")) requests.push(r.url()); });
  await page.goto("/contracts?kind=character&owner=123");
  await expect(page).toHaveURL(/\/login/);
  expect(requests).toEqual([]);
});

test("contract selects support keyboard selection, dismissal and official filters", async ({ page }) => {
  await setup(page);
  await page.goto("/contracts");
  const owner = page.getByRole("combobox", { name: "查看角色" });
  await owner.click();
  await expect(page.getByRole("option", { name: "远航舰长", exact: true })).toHaveAttribute("aria-selected", "true");
  await page.keyboard.press("Escape");
  await expect(owner).toBeFocused();
  await owner.press("ArrowDown");
  await expect(page.getByRole("listbox")).toBeVisible();
  await page.keyboard.press("Home");
  await expect(page.getByRole("option", { name: "远航舰长", exact: true })).toBeFocused();
  await page.keyboard.press("ArrowDown");
  await expect(page.getByRole("option", { name: "工业分队舰长", exact: true })).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(owner).toHaveText("工业分队舰长");
  await expect(page).toHaveURL(/owner=456/);
  await page.getByRole("combobox", { name: "合同类型" }).click();
  await expect(page.getByRole("group", { name: "兼容数据" })).toBeVisible();
  await page.getByRole("option", { name: "借贷", exact: true }).click();
  await expect(page).toHaveURL(/type=loan/);
  await page.getByRole("combobox", { name: "合同类型" }).click();
  await page.getByRole("option", { name: "全部类型", exact: true }).click();
  await expect(page).not.toHaveURL(/type=/);
  await page.getByRole("combobox", { name: "合同状态" }).click();
  await page.getByRole("option", { name: "撤销", exact: true }).click();
  await expect(page).toHaveURL(/status=reversed/);
});

test("opened contract dropdowns fit desktop and phone screens", async ({ page }, info) => {
  await setup(page);
  await page.goto("/contracts");
  mkdirSync("../docs/ui/reviews/contracts-select", { recursive: true });
  for (const width of [1440, 375, 320]) {
    await page.setViewportSize({ width, height: 900 });
    for (const [label, name] of [["查看角色", "owner"], ["合同状态", "status"]]) {
      await page.getByRole("combobox", { name: label }).click();
      const popup = page.getByRole("listbox");
      await expect(popup).toBeVisible();
      const rect = await popup.boundingBox();
      expect(rect!.x).toBeGreaterThanOrEqual(0);
      expect(rect!.x + rect!.width).toBeLessThanOrEqual(width);
      expect(rect!.y + rect!.height).toBeLessThanOrEqual(900);
      await page.screenshot({ path: `../docs/ui/reviews/contracts-select/${info.project.name}-${name}-${width}.png`, animations: "disabled" });
      await page.keyboard.press("Escape");
    }
  }
});

test("contract filters, details and pagination survive return", async ({
  page,
}) => {
  await setup(page);
  await page.goto("/contracts");
  await expect(page.getByRole("table", { name: "合同列表" })).toBeVisible();
  await page.getByRole("button", { name: "下一页合同", exact: true }).click();
  await expect(page.getByText("第 2 页")).toBeVisible();
  await page
    .getByRole("button", { name: "查看合同 9002", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "合同详情", exact: true }),
  ).toBeVisible();
  await expect(page.getByText("蓝图复制品")).toBeVisible();
  await expect(page.getByText("123,456,789,012,345.67").first()).toBeVisible();
  await expect(
    page.getByText("明细尚未同步完成，当前内容可能不完整。"),
  ).toBeVisible();
  await page.getByRole("button", { name: "返回合同列表" }).click();
  await expect(page.getByText("第 2 页")).toBeVisible();
  await page.getByRole("button", { name: "上一页合同", exact: true }).click();
  await expect(page.getByText("第 1 页")).toBeVisible();
  await page
    .getByRole("textbox", { name: "搜索合同", exact: true })
    .fill("找不到");
  await page.getByRole("button", { name: "搜索合同", exact: true }).click();
  await expect(page.getByText("没有匹配的合同")).toBeVisible();
});
test("contract layout desktop and narrow widths", async ({ page }, info) => {
  await setup(page);
  const widths = info.project.name === "desktop" ? [1440] : [375, 320];
  mkdirSync("../docs/ui/reviews/contract-view", { recursive: true });
  for (const width of widths) {
    await page.setViewportSize({ width, height: 960 });
    await page.goto("/contracts");
    await expect(page.getByText(sample.title)).toBeVisible();
    await expect
      .poll(() =>
        page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
      )
      .toBe(true);
    await page.screenshot({
      path: `../docs/ui/reviews/contract-view/list-${width}.png`,
      fullPage: true,
    });
    await page
      .getByRole("button", { name: "查看合同 9001", exact: true })
      .click();
    await expect(page.getByText("三钛合金")).toBeVisible();
    await expect
      .poll(() =>
        page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
      )
      .toBe(true);
    await page.screenshot({
      path: `../docs/ui/reviews/contract-view/detail-${width}.png`,
      fullPage: true,
    });
  }
});
test("scope switches and forbidden details do not leak cached content", async ({
  page,
}) => {
  await setup(page);
  await page.goto("/contracts");
  await page.getByRole("button", { name: "军团合同", exact: true }).click();
  await expect(page.getByRole("combobox", { name: "查看军团" })).toContainText("Glory Navy");
  await page
    .getByRole("button", { name: "查看合同 9001", exact: true })
    .click();
  await expect(page).toHaveURL(/kind=corporation/);
  await expect(page.getByText("Glory Navy", { exact: true })).toBeVisible();
  await page.goto("/contracts?kind=character&owner=123&contract=999");
  await expect(page.getByRole("alert")).toContainText(
    "合同不存在或没有查看权限",
  );
  await expect(page.getByText("三钛合金")).toHaveCount(0);
});

for (const width of [1440, 1024, 375, 320]) {
  test(`generated contract summary at ${width}px`, async ({ page }) => {
    const summary = "提供 三钛合金等 12 种物品 · 换取 伊什塔级蓝图 × 1";
    await page.setViewportSize({ width, height: 1000 });
    await setup(page, { title: "", summary, trade_direction: "exchange" });
    await page.goto("/contracts");
    const link = page.getByRole("button", { name: summary, exact: true });
    await expect(link).toBeVisible();
    await expect(
      page
        .getByRole("row")
        .filter({ has: link })
        .getByText("交换", { exact: true }),
    ).toBeVisible();
    await expect(page.getByText("#9001", { exact: true })).toBeVisible();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBeTruthy();
    mkdirSync("../docs/ui/reviews/contract-summary", { recursive: true });
    await page.screenshot({
      path: `../docs/ui/reviews/contract-summary/list-${width}.png`,
      fullPage: true,
    });
    await link.click();
    await expect(
      page.getByRole("heading", { name: summary, exact: true }),
    ).toBeVisible();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBeTruthy();
    await page.screenshot({
      path: `../docs/ui/reviews/contract-summary/detail-${width}.png`,
      fullPage: true,
    });
  });
}

for (const [direction, label] of [
  ["sell", "出售"],
  ["buy", "求购"],
  ["transport", "运输"],
  ["unknown", "待确认"],
] as const) {
  test(`trade direction ${direction} remains visible with original description`, async ({
    page,
  }) => {
    await setup(page, { trade_direction: direction });
    await page.goto("/contracts");
    if (page.viewportSize()!.width > 1100)
      await expect(
        page.getByRole("columnheader", { name: "交易方向" }),
      ).toBeVisible();
    await expect(
      page
        .getByRole("row")
        .filter({
          has: page.getByRole("button", { name: "查看合同 9001", exact: true }),
        })
        .getByText(label, { exact: true }),
    ).toBeVisible();
    await page
      .getByRole("button", { name: "查看合同 9001", exact: true })
      .click();
    await expect(page.getByRole("heading", { name: "合同详情", exact: true })).toBeVisible();
    await expect(page.getByText(label, { exact: true })).toBeVisible();
  });
}

test("recipient uses acceptor then assignee and preserves public contracts", async ({
  page,
}) => {
  await setup(page, { assignee: entity("789", "指定军团", "corporation") });
  await page.goto("/contracts");
  const row = (id: string) =>
    page
      .getByRole("row")
      .filter({
        has: page.getByRole("button", { name: `查看合同 ${id}`, exact: true }),
      });
  await expect(
    row("9001").getByText("指定军团", { exact: true }),
  ).toBeVisible();
  await expect(
    row("8998").getByText("联盟接收专员", { exact: true }),
  ).toBeVisible();
  await expect(
    row("8998").getByText("工业分队舰长", { exact: true }),
  ).toHaveCount(0);
  await setup(page);
  await page.reload();
  await expect(row("9001").getByText("公开", { exact: true })).toBeVisible();
});

stateLayouts({ name: "contracts", setup: setup, path: "/contracts", endpoint: "**/api/v1/eve/contracts/owners**", empty: { owners: [] }, emptyText: /暂无可查看的角色|No characters available/ });
