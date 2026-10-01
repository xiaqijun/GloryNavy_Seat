import { test, expect } from "@playwright/test";
import { openNavigation } from "./navigation";
import { mkdirSync } from "node:fs";
test("物品估价、比例和多级导航", async ({ page }, info) => {
  let ratio = 10000,
    version = 1;
  await page.route("https://images.evetech.net/**", (r) => r.abort());
  await page.route("**/api/v1/**", async (r) => {
    const p = new URL(r.request().url()).pathname;
    let data: unknown = {};
    if (p === "/api/v1/modules")
      data = [
        "system",
        "identity",
        "eve",
        "access",
        "market",
        "wallet",
        "attendance",
        "skills",
        "fittings",
        "welfare",
        "exchange",
      ].map((id) => ({ id, api_version: 1, version: "0.1.0" }));
    if (p === "/api/v1/identity/session")
      data = {
        authenticated: true,
        session: {
          user_id: "00000000-0000-4000-8000-000000000001",
          character: { id: "123", name: "Pilot" },
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
    if (p === "/api/v1/market/settings") {
      if (r.request().method() === "POST") {
        const input = r.request().postDataJSON();
        expect(input.version).toBe(version);
        expect(r.request().headers()["x-csrf-token"]).toBe("test");
        ratio = input.ratio_bps;
        version++;
        data = { ratio_bps: ratio, version };
      } else
        data = { settings: { ratio_bps: ratio, version }, administrator: true };
    }
    if (p === "/api/v1/market/estimate")
      data = {
        lines: [
          {
            input: "三钛合金 × 1000",
            name: "三钛合金",
            quantity: "1000",
            type_id: "34",
            status: "ready",
            buy: "4010.00",
            mid: "5020.00",
            sell: "6030.00",
            observed_at: "2026-09-19T10:00:00Z",
          },
          {
            input: "未识别物品",
            name: "未识别物品",
            quantity: "1",
            type_id: "0",
            status: "unknown_type",
            buy: null,
            mid: null,
            sell: null,
            observed_at: null,
          },
        ],
        totals: { buy: "4010.00", mid: "5020.00", sell: "6030.00" },
        adjusted: {
          buy: ((4010 * ratio) / 10000).toFixed(2),
          mid: ((5020 * ratio) / 10000).toFixed(2),
          sell: ((6030 * ratio) / 10000).toFixed(2),
        },
        ratio_bps: ratio,
        complete: false,
      };
    return r.fulfill({ json: { data } });
  });
  await page.goto("/appraisal");
  await expect(page.getByRole("heading", { name: "物品估价" })).toBeVisible();
  await page.getByLabel("物品清单").fill("三钛合金 × 1000\n未识别物品");
  await page.getByRole("button", { name: "估价", exact: true }).click();
  await expect(
    page.getByText("5,020.00", { exact: true }).first(),
  ).toBeVisible();
  await expect(
    page.getByText("部分物品报价不完整，以下为已知小计。"),
  ).toBeVisible();
  await page.getByRole("button", { name: "估价比例", exact: true }).click();
  await page.getByLabel("统一比例 / %").fill("80");
  await page.getByRole("button", { name: "保存", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.getByRole("button", { name: "估价", exact: true }).click();
  await expect(page.getByText("4,016.00", { exact: true })).toBeVisible();
  if (info.project.name === "mobile")
    await page.getByRole("button", { name: "菜单", exact: true }).click();
  const nav = await openNavigation(page, "财务与工具");
  await expect(nav.getByRole("button", { name: "财务与工具" })).toHaveAttribute(
    "aria-expanded",
    "true",
  );
  await expect(nav.getByRole("link", { name: "物品估价" })).toHaveAttribute(
    "aria-current",
    "page",
  );
  await nav.getByRole("button", { name: "作战与训练" }).click();
  await expect(nav.getByRole("link", { name: "舰船损失" })).toBeVisible();
  await expect(
    nav.getByRole("link", { name: "合同", exact: true }),
  ).toBeHidden();
  await nav.getByRole("button", { name: "财务与工具" }).focus();
  await page.keyboard.press("Enter");
  await expect(
    nav.getByRole("link", { name: "合同", exact: true }),
  ).toBeVisible();
  mkdirSync("../docs/ui/reviews/market", { recursive: true });
  for (const width of info.project.name === "desktop" ? [1440] : [375, 320]) {
    await page.setViewportSize({ width, height: 1000 });
    await page.screenshot({
      path: `../docs/ui/reviews/market/${width}.png`,
      fullPage: true,
    });
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBeLessThanOrEqual(width);
  }
});
