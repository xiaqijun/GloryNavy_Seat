import { expect, test, type Page } from "@playwright/test";
import { mkdirSync } from "node:fs";

const modules = [
  "system",
  "identity",
  "eve",
  "access",
  "approval",
  "attendance",
  "fittings",
  "skills",
  "welfare",
  "exchange",
  "wallet",
  "market",
  "courier",
  "feedback",
  "community",
  "reminder",
  "sentry",
  "structures",
  "loan",
];

async function setup(
  page: Page,
  role: "administrator" | "member" | "officer",
  enabled = modules,
) {
  await page.route("https://images.evetech.net/**", (route) => route.abort());
  await page.route("**/api/v1/**", (route) => {
    const path = new URL(route.request().url()).pathname;
    let data: unknown;
    if (path === "/api/v1/modules")
      data = enabled.map((id) => ({ id, api_version: 1, version: "0.1.0" }));
    else if (path === "/api/v1/identity/session")
      data = {
        authenticated: true,
        session: {
          user_id: "member",
          character: { id: "123", name: "Pilot" },
          csrf_token: "csrf",
          expires_at: "2099-01-01T00:00:00Z",
        },
      };
    else if (path === "/api/v1/access/me")
      data = {
        administrator: role === "administrator",
        can_manage: role === "administrator",
        can_manage_sync: role === "administrator",
        site_roles: [],
        characters: [],
      };
    else if (path === "/api/v1/approval/context")
      data = {
        allowed: role !== "member",
        sources: [],
        corporations: [],
        people: [],
        unavailable: [],
      };
    else
      return route.fulfill({
        status: 403,
        json: { error: { message: "Unavailable in navigation fixture" } },
      });
    return route.fulfill({ json: { data } });
  });
}

async function openMenu(page: Page) {
  const toggle = page.locator(".navigation-toggle");
  await expect(page.locator("#workspace-navigation")).toBeAttached();
  if (
    (await toggle.isVisible()) &&
    (await toggle.getAttribute("aria-expanded")) !== "true"
  )
    await toggle.click();
  return page.locator("#workspace-navigation");
}

for (const language of ["zh-CN", "en"]) {
  test(`${language}: 分组、当前页、键盘、历史与手机收起`, async ({
    page,
  }, info) => {
    await setup(page, "administrator");
    await page.goto(`/wallet?lang=${language}`);
    const nav = await openMenu(page);
    const english = language === "en";
    await expect(nav.locator(".navigation-group-trigger")).toHaveText(
      english
        ? [
            "Corporation",
            "Combat & training",
            "Assets & services",
            "Benefits & exchange",
            "Administration",
          ]
        : ["军团事务", "作战训练", "资产服务", "福利兑换", "系统管理"],
    );
    await expect(nav.locator(".navigation-primary > a")).toHaveText(
      english ? ["Dashboard", "Approval center"] : ["工作台", "审批中心"],
    );
    await expect(nav.locator(".navigation-footer a")).toHaveAttribute(
      "href",
      "/feedback",
    );
    await expect(nav.locator("#navigation-finance a")).toHaveText(
      english
        ? ["Wallet", "Contracts", "Item appraisal", "Loans", "Courier service"]
        : ["钱包", "合同", "物品估价", "贷款", "快递服务"],
    );
    await expect(nav.locator('a[href="/wallet"]')).toHaveAttribute(
      "aria-current",
      "page",
    );
    const corporation = nav.locator('[aria-controls="navigation-corporation"]');
    await corporation.focus();
    await page.keyboard.press("Enter");
    await expect(corporation).toHaveAttribute("aria-expanded", "true");
    await expect(nav.locator("#navigation-finance")).toBeHidden();
    await expect(nav.locator("#navigation-corporation a")).toHaveText(
      english
        ? [
            "Corporation operations",
            "Corporation attendance",
            "Structure management",
            "Members",
          ]
        : ["军团运营", "军团考勤", "建筑管理", "成员"],
    );
    await page.keyboard.press("Space");
    await expect(corporation).toHaveAttribute("aria-expanded", "false");
    await page.keyboard.press("Tab");
    await expect(
      nav.locator('[aria-controls="navigation-operations"]'),
    ).toBeFocused();
    await page.keyboard.press("Enter");
    await nav.locator('a[href="/skills"]').click();
    await expect(page).toHaveURL(/\/skills$/);
    if (info.project.name !== "desktop")
      await expect(page.locator(".navigation-toggle")).toHaveAttribute(
        "aria-expanded",
        "false",
      );
    await openMenu(page);
    await expect(
      nav.locator('[aria-controls="navigation-operations"]'),
    ).toHaveAttribute("aria-expanded", "true");
    await expect(nav.locator('a[href="/skills"]')).toHaveAttribute(
      "aria-current",
      "page",
    );
    await page.goBack();
    await openMenu(page);
    await expect(
      nav.locator('[aria-controls="navigation-finance"]'),
    ).toHaveAttribute("aria-expanded", "true");
    await expect(nav.locator('a[href="/wallet"]')).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBeLessThanOrEqual(page.viewportSize()!.width);
    const targets = await nav
      .locator("a:visible, button:visible")
      .evaluateAll((els) => els.map((el) => el.getBoundingClientRect().height));
    expect(targets.every((height) => height >= 44)).toBe(true);
    mkdirSync(".local/navigation/screenshots", { recursive: true });
    await page.screenshot({
      path: `.local/navigation/screenshots/${language}-${info.project.name}.png`,
      fullPage: true,
    });
    if (info.project.name !== "desktop") {
      await page.keyboard.press("Escape");
      await expect(page.locator(".navigation-toggle")).toBeFocused();
      await expect(nav).toBeHidden();
      await openMenu(page);
      await nav.locator(".navigation-footer a").focus();
      await page.keyboard.press("Escape");
      await expect(page.locator(".navigation-toggle")).toBeFocused();
      await expect(nav).toBeHidden();
    }
  });

  for (const role of ["member", "officer"] as const) {
    test(`${language}: ${role} 不显示越权入口`, async ({ page }) => {
      await setup(page, role);
      await page.goto(`/wallet?lang=${language}`);
      const nav = await openMenu(page);
      await expect(
        nav.locator('[aria-controls="navigation-administration"]'),
      ).toHaveCount(0);
      for (const path of [
        "/access",
        "/members",
        "/operations",
        "/rewards",
        "/sync",
        "/system",
        "/reminders",
        "/community",
      ])
        await expect(nav.locator(`a[href="${path}"]`)).toHaveCount(0);
      await expect(nav.locator('a[href="/approvals"]')).toHaveCount(
        role === "officer" ? 1 : 0,
      );
      await expect(nav.locator("#navigation-corporation a")).toHaveCount(2);
      await expect(nav.locator("#navigation-benefits a")).toHaveCount(2);
    });
  }

  test(`${language}: 停用模块后隐藏空组与反馈`, async ({ page }) => {
    await setup(page, "member", ["system", "identity", "access", "wallet"]);
    await page.goto(`/wallet?lang=${language}`);
    const nav = await openMenu(page);
    await expect(nav.locator(".navigation-group-trigger")).toHaveCount(1);
    await expect(nav.locator(".navigation-footer")).toHaveCount(0);
    await expect(nav.locator("a")).toHaveCount(2);
    await expect(nav.locator('a[href="/wallet"]')).toBeVisible();
  });
}
