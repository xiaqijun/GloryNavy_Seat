import { expect, test, type Page } from "@playwright/test";
import { mkdirSync } from "node:fs";

async function setup(
  page: Page,
  role: "member" | "administrator" | "anonymous" = "member",
) {
  const requests: string[] = [];
  await page.route("https://images.evetech.net/**", (route) => route.abort());
  await page.route("**/api/v1/**", (route) => {
    const path = new URL(route.request().url()).pathname;
    requests.push(path);
    const data =
      path === "/api/v1/identity/session"
        ? {
            authenticated: role !== "anonymous",
            session:
              role === "anonymous"
                ? null
                : {
                    user_id: "guide-fixture",
                    character: { id: "123", name: "Guide Pilot" },
                    csrf_token: "fixture",
                    expires_at: "2099-01-01T00:00:00Z",
                  },
          }
        : path === "/api/v1/modules"
          ? ["system", "identity", "access", "approval"].map((id) => ({
              id,
              api_version: 1,
              version: "0.1.0",
            }))
          : path === "/api/v1/access/me"
            ? {
                administrator: role === "administrator",
                can_manage: role === "administrator",
                site_roles: [],
                characters: [],
              }
            : path === "/api/v1/approval/context"
              ? {
                  allowed: role === "administrator",
                  sources: [],
                  corporations: [],
                  people: [],
                  unavailable: [],
                }
              : undefined;
    return data === undefined
      ? route.fulfill({
          status: 403,
          json: { error: { message: "Not in guide fixture" } },
        })
      : route.fulfill({ json: { data } });
  });
  return requests;
}

for (const locale of ["zh-CN", "en"]) {
  test(`${locale}: help search, bookmarks, history and keyboard`, async ({
    page,
  }, info) => {
    const requests = await setup(page);
    const en = locale === "en";
    await page.goto(`/help?lang=${locale}&topic=exchange`);
    const title = page.locator("#help-article-title");
    await expect(title).toContainText(en ? "Redeem" : "兑换奖励");
    const input = page.getByLabel(en ? "Search help" : "搜索帮助", {
      exact: true,
    });
    await input.fill(en ? "80%" : "八折");
    await expect(page.locator(".help-results")).toBeVisible();
    await expect(page.locator(".help-article")).toHaveCount(0);
    const result = page.locator('.help-result[href*="topic=loans"]');
    await result.focus();
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(/topic=loans/);
    await expect(title).toBeFocused();
    await expect(page.locator(".help-article")).toContainText("80%");
    await page.goBack();
    await expect(page).toHaveURL(/topic=exchange/);
    await expect(title).toContainText(en ? "Redeem" : "兑换奖励");
    await input.fill("no-such-topic-123456");
    await expect(page.getByRole("status")).toHaveText(
      en ? "Found 0 topics" : "找到 0 个主题",
    );
    await page
      .getByRole("button", {
        name: en ? "Clear search" : "清除搜索",
        exact: true,
      })
      .click();
    await expect(input).toBeFocused();
    await expect(title).toBeVisible();
    if (info.project.name !== "desktop") {
      const select = page.getByRole("combobox", {
        name: en ? "Help topics" : "帮助目录",
      });
      await select.click();
      await page
        .getByRole("option")
        .filter({ hasText: en ? "Wallet" : "钱包与交税记录" })
        .click();
    } else
      await page.locator('.help-directory a[href*="topic=wallet"]').click();
    await expect(page).toHaveURL(/topic=wallet/);
    await expect(title).toBeFocused();
    expect(
      requests.filter(
        (path) =>
          ![
            "/api/v1/modules",
            "/api/v1/identity/session",
            "/api/v1/access/me",
            "/api/v1/approval/context",
          ].includes(path),
      ),
    ).toEqual([]);
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBeLessThanOrEqual(page.viewportSize()!.width);
    if (en)
      expect(await page.locator(".help-page").innerText()).not.toMatch(
        /[\u3400-\u9fff]/,
      );
    mkdirSync("../docs/ui/reviews/help-20261020", { recursive: true });
    await page.evaluate(() => window.scrollTo({ top: 0, behavior: "instant" }));
    await page.mouse.move(0, 0);
    await page.screenshot({
      path: `../docs/ui/reviews/help-20261020/${locale}-${info.project.name}.png`,
      fullPage: true,
    });
    await page.emulateMedia({ media: "print" });
    await expect(page.locator(".sidebar")).toBeHidden();
    await expect(page.locator(".help-search")).toBeHidden();
    await expect(title).toBeVisible();
  });
}

test("unknown topic falls back; static management guide grants no extra permissions", async ({
  page,
}) => {
  await setup(page);
  await page.goto("/help?topic=unknown");
  await expect(page.locator("#help-article-title")).toHaveText(
    "登录与绑定角色",
  );
  await page.goto("/help?topic=administration");
  await expect(page.locator("#help-article-title")).toHaveText(
    "配置角色与提醒",
  );
  await expect(
    page.locator('#workspace-navigation a[href="/access"]'),
  ).toHaveCount(0);
  await expect(
    page.locator('#workspace-navigation a[href="/help"]'),
  ).toBeAttached();
});

test("administrator can open guide; anonymous visitor returns to public home", async ({
  page,
}) => {
  await setup(page, "administrator");
  await page.goto("/help?topic=officers");
  await expect(page.locator("#help-article-title")).toHaveText(
    "审批与合同发放",
  );
  await page.unrouteAll({ behavior: "wait" });
  await setup(page, "anonymous");
  await page.goto("/help");
  await expect(page).toHaveURL(/\/$/);
  await expect(page.locator(".help-page")).toHaveCount(0);
});
