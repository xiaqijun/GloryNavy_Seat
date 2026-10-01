import { stateLayouts } from "./state-layout";
import { test, expect, type Page } from "@playwright/test";
import { mkdirSync } from "node:fs";
async function setup(page: Page, refType = "player_donation") {
  await page.route("https://images.evetech.net/**", (r) => r.abort());
  await page.route("**/api/v1/**", (r) => {
    const url = new URL(r.request().url()),
      p = url.pathname;
    let data: unknown = {};
    if (p === "/api/v1/modules")
      data = ["system", "identity", "eve", "access", "wallet"].map((id) => ({
        id,
        api_version: 1,
        version: "0.1.0",
      }));
    if (p === "/api/v1/identity/session")
      data = {
        authenticated: true,
        session: {
          user_id: "00000000-0000-4000-8000-000000000001",
          character: { id: "123", name: "Hajimi1" },
          csrf_token: "test",
          expires_at: "2099-01-01T00:00:00Z",
        },
      };
    if (p === "/api/v1/access/me")
      data = {
        administrator: true,
        can_manage: true,
        can_manage_sync: true,
        site_roles: [],
        characters: [],
      };
    if (p === "/api/v1/wallet/context")
      data = {
        owners: [
          {
            kind: "character",
            id: "123",
            name: "Hajimi1",
            divisions: [0],
            journal: true,
            transactions: true,
          },
          {
            kind: "corporation",
            id: "10",
            name: "Glory Navy",
            divisions: [1, 3],
            journal: true,
            transactions: true,
          },
        ],
      };
    if (p === "/api/v1/wallet/records") {
      const division = Number(url.searchParams.get("division")),
        base = { id: "0", division, observed_at: "2026-09-16T09:00:00Z" };
      const part = url.searchParams.get("part");
      data = {
        items:
          part === "balance"
            ? [{ ...base, balance: "1234567890.67" }]
            : part === "divisions"
              ? [{ ...base, name: "军团运营" }]
              : part === "transactions"
                ? [
                    {
                      ...base,
                      id: "9007199254740993",
                      date: "2026-09-16T08:00:00Z",
                      type_id: "34",
                      type_name: "三钛合金",
                      unit_price: "4.25",
                      quantity: "125000",
                      client_id: "456",
                      journal_ref_id: "9007199254740992",
                      is_buy: true,
                      is_personal: true,
                    },
                  ]
                : [
                    {
                      ...base,
                      id: "9007199254740993",
                      date: "2026-09-16T08:00:00Z",
                      ref_type: refType,
                      description:
                        "从成员收到的转账。完整说明应保留在详情弹窗中。",
                      reason: "9 月军团活动拨款",
                      first_party_id: "456",
                      second_party_id: "123",
                      amount: "250000000.00",
                      balance: "1234567890.67",
                    },
                    {
                      ...base,
                      id: "9007199254740992",
                      date: "2026-09-15T08:00:00Z",
                      ref_type: "market_transaction",
                      description: "装配物资采购",
                      first_party_id: "123",
                      second_party_id: "456",
                      amount: "-123456789012345.67",
                      balance: "-9876543210.01",
                    },
                  ],
        names: { "123": "Hajimi1", "456": "Nuter Zero" },
        next_cursor: "",
      };
    }
    return r.fulfill({ json: { data } });
  });
}
test("钱包筛选、精确金额和统一详情弹窗", async ({ page }, info) => {
  await setup(page);
  await page.goto("/wallet");
  mkdirSync("../docs/ui/reviews/wallet", { recursive: true });
  await page.screenshot({
    path: `../docs/ui/reviews/wallet/initial-${info.project.name}.png`,
    fullPage: true,
  });
  await expect(
    page.getByRole("heading", { name: "钱包", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("-123,456,789,012,345.67", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "查看记录 9007199254740993" }).click();
  const modal = page.getByRole("dialog", { name: "流水详情" });
  await expect(
    modal.getByText("从成员收到的转账。完整说明应保留在详情弹窗中。"),
  ).toBeVisible();
  await modal.focus();
  await page.keyboard.press("Escape");
  await expect(modal).toHaveCount(0);
  await page.getByRole("button", { name: "更多筛选", exact: true }).click();
  await page.getByLabel("参与方 ID", { exact: true }).fill("456");
  const query = page.waitForRequest(
    (r) =>
      r.url().includes("/wallet/records?") &&
      new URL(r.url()).searchParams.get("party_id") === "456",
  );
  await page.getByRole("button", { name: "查询记录", exact: true }).click();
  await query;
  await page.getByRole("button", { name: "更多筛选", exact: true }).click();
  mkdirSync("../docs/ui/reviews/wallet", { recursive: true });
  await page.screenshot({
    path: `../docs/ui/reviews/wallet/${info.project.name}.png`,
    fullPage: true,
  });
  await page.getByRole("button", { name: "市场交易", exact: true }).click();
  await expect(page.getByText("三钛合金", { exact: true })).toBeVisible();
  await expect(
    page
      .getByRole("region", { name: "钱包记录表格" })
      .getByText("买入", { exact: true }),
  ).toBeVisible();
  await page.getByRole("combobox", { name: "钱包归属" }).click();
  await page.getByRole("option", { name: "Glory Navy", exact: true }).click();
  await expect(page.getByRole("combobox", { name: "钱包分部" })).toBeVisible();
  await page.getByRole("combobox", { name: "钱包分部" }).click();
  await expect(
    page.getByRole("option", { name: "分部 2", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("option", { name: "分部 3", exact: true }).click();
  for (const width of [375, 320]) {
    await page.setViewportSize({ width, height: 812 });
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
      path: `../docs/ui/reviews/wallet/${info.project.name}-${width}.png`,
      fullPage: true,
    });
  }
});
test("同一钱包刷新保留记录，切换钱包清理旧记录", async ({ page }) => {
  await setup(page);
  await page.goto("/wallet");
  await expect(
    page.getByText("9 月军团活动拨款", { exact: true }),
  ).toBeVisible();
  let release!: () => void;
  const pending = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/v1/wallet/records?**", async (r) => {
    if (new URL(r.request().url()).searchParams.get("part") === "journal")
      await pending;
    await r.fallback();
  });
  await page.getByRole("button", { name: "刷新记录", exact: true }).click();
  await expect(
    page.getByText("9 月军团活动拨款", { exact: true }),
  ).toBeVisible();
  await page.getByRole("combobox", { name: "钱包归属" }).click();
  await page.getByRole("option", { name: "Glory Navy", exact: true }).click();
  await expect(page.getByText("9 月军团活动拨款", { exact: true })).toHaveCount(
    0,
  );
  release();
  await expect(
    page.getByText("9 月军团活动拨款", { exact: true }),
  ).toBeVisible();
});

test("中英文切换保留路由和原始数据，钱包弹窗及菜单同步切换", async ({
  page,
}, info) => {
  await setup(page);
  await page.goto("/wallet?member=42");
  await expect(
    page.getByRole("heading", { name: "钱包", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "切换为英文", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Wallet", exact: true }),
  ).toBeVisible();
  expect(new URL(page.url()).searchParams.get("member")).toBe("42");
  await expect(page.locator("html")).toHaveAttribute("lang", "en");
  await expect(
    page.getByText("9 月军团活动拨款", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("Player Donation", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("-123,456,789,012,345.67", { exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "View record 9007199254740993" })
    .click();
  const dialog = page.getByRole("dialog", { name: "Journal details" });
  await expect(
    dialog.getByText("Original description", { exact: true }),
  ).toBeVisible();
  await expect(
    dialog.getByText("从成员收到的转账。完整说明应保留在详情弹窗中。", {
      exact: true,
    }),
  ).toBeVisible();
  await dialog.getByRole("button", { name: "Close", exact: true }).click();
  await page.reload();
  await expect(
    page.getByRole("heading", { name: "Wallet", exact: true }),
  ).toBeVisible();
  for (const width of [1280, 375]) {
    await page.setViewportSize({ width, height: 900 });
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
      path: `../.local/i18n-wallet-${info.project.name}-${width}.png`,
      fullPage: true,
    });
  }
  await page
    .getByRole("button", { name: "Switch to Chinese", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "钱包", exact: true }),
  ).toBeVisible();
  await expect(page.locator("html")).toHaveAttribute("lang", "zh-CN");
});

test("登录页可切换语言，切换后保留登录错误查询参数", async ({ page }) => {
  await setup(page);
  await page.route("**/api/v1/identity/session", (r) =>
    r.fulfill({ json: { data: { authenticated: false, session: null } } }),
  );
  await page.route("**/api/v1/eve/login-status", (r) =>
    r.fulfill({ json: { data: { configured: true } } }),
  );
  await page.goto("/login?source=test");
  await page.getByRole("button", { name: "切换为英文", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Sign in with EVE Online", exact: true }),
  ).toBeVisible();
  expect(new URL(page.url()).searchParams.get("source")).toBe("test");
  await expect(
    page.getByRole("button", { name: "Switch to Chinese", exact: true }),
  ).toBeVisible();
});

test("官方术语中英展示保留原始流水代码且窄屏不溢出", async ({ page }, info) => {
  await setup(page, "ess_escrow_transfer");
  await page.goto("/wallet");
  await expect(
    page.getByText("事件监测装置保证金支付", { exact: true }),
  ).toBeVisible();
  await page.setViewportSize({ width: 375, height: 900 });
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          document.documentElement.scrollWidth <=
          document.documentElement.clientWidth,
      ),
    )
    .toBe(true);
  await page.locator(".wallet-table-scroll").evaluate((element) => {
    element.scrollLeft = 0;
  });
  await page.mouse.move(0, 0);
  mkdirSync("../.local/terminology", { recursive: true });
  await page.screenshot({
    path: `../.local/terminology/wallet-zh-${info.project.name}.png`,
    fullPage: true,
  });
  await page.getByRole("button", { name: "查看记录 9007199254740993" }).click();
  await expect(
    page.getByRole("dialog").getByText("ess_escrow_transfer", { exact: true }),
  ).toBeVisible();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "关闭", exact: true })
    .click();
  await page.getByRole("button", { name: "切换为英文", exact: true }).click();
  await expect(
    page.getByText("ESS Escrow Payment", { exact: true }),
  ).toBeVisible();
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
    path: `../.local/terminology/wallet-en-${info.project.name}.png`,
    fullPage: true,
  });
});

stateLayouts({ name: "wallet", setup: setup, path: "/wallet", endpoint: "**/api/v1/wallet/context**", empty: { owners: [] }, emptyText: /暂无可查看的钱包|No wallets/ });
