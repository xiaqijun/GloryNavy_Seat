import { test, expect, type Page } from "@playwright/test";
import { mkdirSync } from "node:fs";

async function quantityShop(
  page: Page,
  locale: "zh-CN" | "en",
  fail = false,
  budget = 10000,
) {
  await page.route("https://images.evetech.net/**", (r) => r.abort());
  const requests: Record<string, unknown>[] = [];
  let reserved = 0,
    stock = 6;
  const orders: Record<string, unknown>[] = [];
  const content = {
    fittings: [
      { fitting_id: "1", name: "舰船配装", ship_type_id: "34", quantity: 2 },
    ],
    items: [
      {
        type_id: "35",
        name: "附加弹药与补给物品名称很长仍需完整显示",
        quantity: 1000000,
      },
    ],
    isk_minor: 12345,
  };
  await page.route("**/api/v1/**", async (r) => {
    const path = new URL(r.request().url()).pathname;
    let data: unknown;
    if (path.endsWith("/modules"))
      data = ["system", "identity", "eve", "access", "exchange"].map((id) => ({
        id,
        api_version: 1,
        version: "0.1.0",
      }));
    else if (path.endsWith("/identity/session"))
      data = {
        authenticated: true,
        session: {
          user_id: "user",
          character: { id: "123", name: "Hajimi1" },
          csrf_token: "csrf",
          expires_at: "2099-01-01T00:00:00Z",
        },
      };
    else if (path.endsWith("/access/me"))
      data = {
        administrator: true,
        can_manage: false,
        can_manage_sync: false,
        site_roles: [],
        characters: [],
      };
    else if (path.endsWith("/exchange/context"))
      data = { characters: [{ id: "123", name: "Hajimi1" }], sources: [] };
    else if (path.endsWith("/exchange/wallet"))
      data = { items: [], next_cursor: "" };
    else if (path.endsWith("/rewards/claim")) {
      expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
      const c = r.request().postDataJSON();
      requests.push(c);
      if (fail && requests.length === 1)
        return r.fulfill({
          status: 503,
          json: {
            error: {
              code: "exchange_unavailable",
              message: "暂时失败，请重试",
            },
          },
        });
      const units = c.units;
      const coins_minor = 401 * units;
      reserved += coins_minor;
      stock -= units;
      orders.push({
        id: "1",
        version: "1",
        type_id: "34",
        name: "混合奖励礼包",
        quantity: units,
        units,
        recipient_id: "123",
        recipient_name: "Hajimi1",
        coins_minor,
        isk_per_coin: 100,
        isk_value: 401 * units,
        state: "pending",
        can_request_cancel: true,
        reference: "EX-20261019-000123",
        delivery: {
          status: "waiting_contract",
          checked_at: null,
          contracts: [],
        },
        content: {
          fittings: content.fittings.map((f) => ({
            ...f,
            quantity: f.quantity * units,
          })),
          items: content.items.map((i) => ({
            ...i,
            quantity: i.quantity * units,
          })),
          isk_minor: content.isk_minor * units,
        },
        note: "",
        created_at: "2026-10-19T00:00:00Z",
      });
      data = { id: "1" };
    } else if (path.endsWith("/handoff"))
      data = {
        reference: "EX-20261019-000123",
        recipient_id: "123",
        recipient_name: "Hajimi1",
        state: "pending",
        amount: String(
          Math.floor((Number(orders[0]?.units || 1) * 12345) / 100),
        ),
      };
    else if (path.endsWith("/rewards/orders"))
      data = { items: orders, next_cursor: "" };
    else if (path.endsWith("/rewards"))
      data = {
        admin: true,
        isk_per_coin: 100,
        version: "1",
        earned_minor: budget,
        reserved_minor: reserved,
        spent_minor: 0,
        available_minor: budget - reserved,
        rewards: [
          {
            id: "1",
            type_id: "34",
            name: "混合奖励礼包",
            quantity: 1,
            isk_value: 401,
            coins_minor: 401,
            stock,
            enabled: true,
            version: "1",
            content,
          },
        ],
        next_cursor: "",
      };
    else
      return r.fulfill({
        status: 404,
        json: { error: { code: "not_found", message: path } },
      });
    return r.fulfill({ json: { data } });
  });
  await page.goto("/exchange");
  if (locale === "en")
    await page.getByRole("button", { name: "切换为英文", exact: true }).click();
  await page
    .getByRole("button", {
      name: locale === "en" ? "Redeem" : "兑换",
      exact: true,
    })
    .click();
  return requests;
}

for (const locale of ["zh-CN", "en"] as const) {
  const label = locale === "en" ? "Packs" : "兑换份数";
  const confirm = locale === "en" ? "Confirm conversion" : "确认兑换";
  test(`${locale} multi-pack totals and one order`, async ({ page }, info) => {
    const requests = await quantityShop(page, locale);
    const dialog = page.getByRole("dialog");
    await dialog.getByRole("spinbutton", { name: label }).fill("3");
    await expect(dialog.locator(".exchange-refund-summary")).toContainText(
      "12.03",
    );
    await expect(dialog.getByText("× 6", { exact: true })).toBeVisible();
    await expect(dialog.getByText("× 3000000", { exact: true })).toBeVisible();
    await expect(dialog.getByText("370.35", { exact: true })).toBeVisible();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    mkdirSync("../.local/exchange-quantity/screenshots", { recursive: true });
    await page.screenshot({
      path: `../.local/exchange-quantity/screenshots/claim-${locale}-${info.project.name}.png`,
      fullPage: true,
    });
    await dialog.getByRole("button", { name: confirm, exact: true }).click();
    await expect(dialog).toHaveCount(0);
    expect(requests).toHaveLength(1);
    expect(requests[0].units).toBe(3);
    await expect(page.locator(".exchange-order-row")).toContainText(
      locale === "en" ? "3 packs" : "3 份",
    );
    await page
      .getByText(locale === "en" ? "Reward contents" : "奖励内容", {
        exact: true,
      })
      .click();
    await expect(page.locator(".exchange-order-row")).toContainText("3000000");
    await page
      .getByRole("button", {
        name: locale === "en" ? "Contract information" : "合同信息",
        exact: true,
      })
      .click();
    await expect(
      page.getByRole("dialog").locator('input[value="370"]'),
    ).toBeVisible();
    await expect(
      page.getByRole("dialog").locator('input[value="EX-20261019-000123"]'),
    ).toBeVisible();
  });
  test(`${locale} rejects fractions zero and over-stock`, async ({ page }) => {
    const requests = await quantityShop(page, locale);
    const dialog = page.getByRole("dialog");
    const input = dialog.getByRole("spinbutton", { name: label });
    for (const value of ["0", "-1", "1.5", "7", ""]) {
      await input.fill(value);
      await expect(
        dialog.getByRole("button", { name: confirm, exact: true }),
      ).toBeDisabled();
    }
    await input.fill("6");
    await expect(
      dialog.getByRole("button", { name: confirm, exact: true }),
    ).toBeEnabled();
    expect(requests).toHaveLength(0);
  });
  test(`${locale} affordable limit and default single pack`, async ({
    page,
  }) => {
    const requests = await quantityShop(page, locale, false, 900);
    const dialog = page.getByRole("dialog");
    const input = dialog.getByRole("spinbutton", { name: label });
    await expect(input).toHaveValue("1");
    await expect(input).toHaveAttribute("max", "2");
    await input.fill("3");
    await expect(
      dialog.getByRole("button", { name: confirm, exact: true }),
    ).toBeDisabled();
    await input.fill("1");
    await dialog.getByRole("button", { name: confirm, exact: true }).click();
    await expect(dialog).toHaveCount(0);
    expect(requests[0].units).toBe(1);
  });
  test(`${locale} retry preserves quantity and idempotency`, async ({
    page,
  }) => {
    const requests = await quantityShop(page, locale, true);
    const dialog = page.getByRole("dialog");
    const input = dialog.getByRole("spinbutton", { name: label });
    await input.fill("2");
    await dialog.getByRole("button", { name: confirm, exact: true }).click();
    await expect(dialog.getByRole("alert")).toBeVisible();
    await expect(input).toHaveValue("2");
    await dialog.getByRole("button", { name: confirm, exact: true }).click();
    await expect(dialog).toHaveCount(0);
    expect(requests).toHaveLength(2);
    expect(requests[0]).toEqual(requests[1]);
  });
}
