import { test, expect, type Page } from "@playwright/test";
import { mkdirSync } from "node:fs";

async function setup(page: Page) {
  const reads: URL[] = [];
  await page.route("https://images.evetech.net/**", (route) => route.abort());
  await page.route("**/api/v1/**", (route) => {
    const url = new URL(route.request().url());
    reads.push(url);
    const p = url.pathname;
    let data: unknown;
    if (p === "/api/v1/modules") data = ["system", "identity", "eve", "access", "approval", "wallet"].map((id) => ({ id, version: "0.1.0", api_version: 1 }));
    else if (p === "/api/v1/identity/session") data = { authenticated: true, session: { user_id: "00000000-0000-4000-8000-000000000001", character: { id: "123", name: "Pilot" }, csrf_token: "test", expires_at: "2099-01-01T00:00:00Z" } };
    else if (p === "/api/v1/access/me") data = { administrator: true, can_manage: true, can_manage_sync: true, site_roles: [], characters: [] };
    else if (p === "/api/v1/eve/contracts/owners") data = { owners: [{ kind: "character", id: "123", name: "Pilot" }] };
    else if (p === "/api/v1/eve/contracts/character/123") data = { items: [], next_cursor: "" };
    else if (p === "/api/v1/access/members") data = { items: [], next: "" };
    else if (p === "/api/v1/approval/context") data = { allowed: true, sources: ["welfare"], corporations: [], people: [], unavailable: [] };
    else if (p === "/api/v1/approval/items") data = { items: [], counts: { pending: 0, information: 0, fulfillment: 0, exceptions: 0, history: 0 }, next_cursor: "", unavailable: [] };
    else if (p === "/api/v1/wallet/context") data = { owners: [{ kind: "character", id: "123", name: "Pilot", divisions: [0], journal: true, transactions: true }] };
    else if (p === "/api/v1/wallet/records") data = { items: [], names: {}, next_cursor: "" };
    else return route.fulfill({ status: 403, json: { error: { message: "Unavailable in search fixture" } } });
    return route.fulfill({ json: { data } });
  });
  return reads;
}

for (const lang of ["zh-CN", "en"]) {
  for (const entry of [
    { path: "contracts", selector: "#contract-search", filters: "owner=123&type=item_exchange&status=outstanding&before=800&trail=800", cursor: "before" },
    { path: "members", selector: "#member-search", filters: "after=00000000-0000-4000-8000-000000000002&trail=previous", cursor: "after" },
    { path: "approvals", selector: ".approval-search input", filters: "view=pending&kind=solo&cursor=previous", cursor: "cursor" },
  ]) {
    test(`${lang} ${entry.path} 搜索、历史返回、清空与输入法`, async ({ page }, info) => {
      await setup(page);
      await page.goto(`/${entry.path}?lang=${lang}&q=旧搜索&${entry.filters}`);
      const input = page.locator(entry.selector);
      await expect(input).toHaveValue("旧搜索");
      await input.fill("  三钛合金   Pilot  ");
      expect(await input.evaluate((node) => {
        const event = new KeyboardEvent("keydown", { key: "Enter", keyCode: 229, isComposing: true, bubbles: true, cancelable: true });
        node.dispatchEvent(event);
        return event.defaultPrevented;
      })).toBe(true);
      expect(new URL(page.url()).searchParams.get("q")).toBe("旧搜索");
      await input.press("Enter");
      await expect.poll(() => new URL(page.url()).searchParams.get("q")).toBe("三钛合金 Pilot");
      await expect(input).toHaveValue("三钛合金 Pilot");
      expect(new URL(page.url()).searchParams.has(entry.cursor)).toBe(false);
      if (entry.path !== "approvals") expect(new URL(page.url()).searchParams.has("trail")).toBe(false);
      await input.fill("未提交的内容");
      await page.goBack();
      await expect(input).toHaveValue("旧搜索");
      await page.goForward();
      await expect(input).toHaveValue("三钛合金 Pilot");
      await page.getByRole("button", { name: lang === "en" ? "Clear search" : "清除搜索", exact: true }).click();
      await expect(input).toHaveValue("");
      await expect(input).toBeFocused();
      await expect.poll(() => new URL(page.url()).searchParams.has("q")).toBe(false);
      if (entry.path === "contracts") {
        expect(new URL(page.url()).searchParams.get("type")).toBe("item_exchange");
        expect(new URL(page.url()).searchParams.get("status")).toBe("outstanding");
      }
      if (entry.path === "approvals") expect(new URL(page.url()).searchParams.get("kind")).toBe("solo");
      await input.fill("三钛合金");
      await input.press("Enter");
      mkdirSync(".local/search/screenshots", { recursive: true });
      await page.screenshot({ path: `.local/search/screenshots/${entry.path}-${lang}-${info.project.name}.png`, fullPage: true });
      expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(page.viewportSize()!.width);
      const clear = page.getByRole("button", { name: lang === "en" ? "Clear search" : "清除搜索", exact: true });
      expect((await clear.boundingBox())!.width).toBeGreaterThanOrEqual(44);
    });
  }

  test(`${lang} 钱包搜索保留日期筛选并清空关键词`, async ({ page }, info) => {
    const reads = await setup(page);
    await page.goto(`/wallet?lang=${lang}`);
    const input = page.locator(".wallet-search input");
    await expect(input).toBeVisible();
    await page.getByRole("button", { name: lang === "en" ? "More filters" : "更多筛选", exact: true }).click();
    await page.locator('input[type="date"]').first().fill("2026-10-01");
    await page.locator('input[type="date"]').nth(1).fill("2026-10-02");
    await input.fill("  Buyer   Pilot  ");
    await input.press("Enter");
    await expect.poll(() => reads.some((u) => u.pathname.endsWith("/records") && u.searchParams.get("search") === "Buyer Pilot" && !!u.searchParams.get("from"))).toBe(true);
    await expect(page.locator('input[type="date"]').first()).toHaveValue("2026-10-01");
    await input.press("Enter");
    await expect(page.locator('input[type="date"]').nth(1)).toHaveValue("2026-10-02");
    await page.getByRole("button", { name: lang === "en" ? "Clear search" : "清除搜索", exact: true }).click();
    await expect(input).toHaveValue("");
    await expect.poll(() => reads.some((u) => u.pathname.endsWith("/records") && !u.searchParams.get("search") && !!u.searchParams.get("from"))).toBe(true);
    await input.fill("三钛合金");
    mkdirSync(".local/search/screenshots", { recursive: true });
    await page.screenshot({ path: `.local/search/screenshots/wallet-${lang}-${info.project.name}.png`, fullPage: true });
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(page.viewportSize()!.width);
  });
}
