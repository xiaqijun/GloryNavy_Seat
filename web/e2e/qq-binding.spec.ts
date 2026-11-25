import { test, expect } from "@playwright/test";
import { mockCharacterSync } from "./sync-fixture";

for (const locale of ["zh-CN", "en"]) {
  test(`QQ binding lifecycle ${locale}`, async ({ page, context }, info) => {
    const en = locale === "en";
    await page.addInitScript((value) => localStorage.setItem("glorynavy.locale", value), locale);
    await context.grantPermissions(["clipboard-read", "clipboard-write"]);
    await page.route("https://images.evetech.net/**", r => r.abort());
    let confirmed = false;
    let generation = 0;
    let fail = true;
    let expire = false;
    await page.route("**/api/v1/**", r => {
      const path = new URL(r.request().url()).pathname;
      let data: unknown = {};
      if (path === "/api/v1/modules") data = ["system", "identity", "eve", "access", "community"].map(id => ({ id, api_version: 1, version: "0.1.0" }));
      if (path === "/api/v1/identity/session") data = { authenticated: true, session: { user_id: "member", character: { id: "101", name: "Pilot" }, csrf_token: "fixture-csrf", expires_at: "2099-01-01T00:00:00Z" } };
      if (path === "/api/v1/identity/characters") data = { characters: [{ id: "101", name: "Pilot", status: "active", is_main: true }] };
      if (path === "/api/v1/eve/status") data = { configured: true };
      if (path === "/api/v1/access/me") data = { administrator: false, site_roles: [], characters: [] };
      if (path === "/api/v1/community/profile") data = { version: "1", complete: true, qq: { value: "123456789", version: "1", confirmation: confirmed ? "confirmed" : "pending" }, kook: { value: "Pilot", version: "1", confirmation: "pending" } };
      if (path === "/api/v1/community/qq/group/options") data = { items: [] };
      if (path === "/api/v1/community/qq/group/application") data = null;
      if (path === "/api/v1/community/qq/challenge") {
        expect(r.request().method()).toBe("POST");
        expect(r.request().headers()["x-csrf-token"]).toBe("fixture-csrf");
        expect(r.request().postData()).toBeNull();
        if (fail) return r.fulfill({ status: 503, json: { error: { message: "Fixture unavailable" } } });
        generation++;
        data = { code: generation === 1 ? "ABCDEFGH" : "JKLMNPQR", expires_at: new Date(Date.now() + (expire ? 1200 : 600000)).toISOString() };
      }
      return r.fulfill({ json: { data } });
    });
    await mockCharacterSync(page);
    await page.goto("/account");
    const button = page.getByRole("button", { name: en ? "Generate QQ binding code" : "生成 QQ 绑定码", exact: true });
    await expect(button).toBeVisible();
    await expect(page.getByRole("button", { name: en ? "Generate group admission code" : "生成入群绑定码", exact: true })).toHaveCount(0);
    await button.focus();
    await page.keyboard.press("Enter");
    await expect(page.getByRole("alert").filter({ hasText: "Fixture unavailable" })).toBeVisible();
    fail = false;
    await button.click();
    await expect(page.locator(".community-qq-command code")).toHaveText("ABCDEFGH");
    await page.getByRole("button", { name: en ? "Copy verification code" : "复制验证码", exact: true }).click();
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe("ABCDEFGH");
    await expect(page.locator(".community-qq-binding p")).toHaveCount(0);
    await expect(page.locator(".community-binding").first().getByText(en ? "Pending confirmation" : "待确认", { exact: true })).toBeVisible();
    for (const width of info.project.name === "desktop" ? [1280, 375, 320] : [393]) {
      await page.setViewportSize({ width, height: 850 });
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      await expect(page.locator(".community-qq-command code")).toBeVisible();
      await page.locator(".community-card").screenshot({ path: `../.local/qq-group-code/binding-${locale}-${width}.png` });
    }
    expire = true;
    await page.getByRole("button", { name: en ? "Generate again" : "重新生成", exact: true }).click();
    await expect(page.getByText(en ? "This binding code has expired. Generate a new one." : "绑定码已过期，请重新生成", { exact: true })).toBeVisible();
    await expect(page.locator(".community-qq-command code")).toHaveCount(0);
    expire = false;
    await page.getByRole("button", { name: en ? "Generate again" : "重新生成", exact: true }).click();
    await expect(page.locator(".community-qq-command code")).toHaveText("JKLMNPQR");
    confirmed = true;
    await expect(page.locator(".community-qq-binding")).toHaveCount(0, { timeout: 10000 });
    await expect(page.locator(".community-binding").first().getByText(en ? "Confirmed" : "已确认", { exact: true })).toBeVisible();
  });

  test(`QQ group admission waits for corporation membership ${locale}`, async ({ page }) => {
    const en = locale === "en";
    await page.addInitScript(value => localStorage.setItem("glorynavy.locale", value), locale);
    await page.route("https://images.evetech.net/**", route => route.abort());
    const group = { group_openid: "fixture-group", label: "Fixture", enabled: true, join_url: "https://qm.qq.com/q/fixture" };
    const application = {
      id: 1, group_openid: group.group_openid, qq_number: "123456789",
      created_at: "2026-11-25T10:00:00Z", expires_at: "2026-11-25T10:15:00Z",
    };
    let status = "";
    await page.route("**/api/v1/**", route => {
      const request = route.request();
      const path = new URL(request.url()).pathname;
      let data: unknown = {};
      let httpStatus = 200;
      if (path === "/api/v1/modules") data = ["system", "identity", "eve", "access", "community"].map(id => ({ id, api_version: 1, version: "0.1.0" }));
      if (path === "/api/v1/identity/session") data = { authenticated: true, session: { user_id: "member", character: { id: "101", name: "Pilot" }, csrf_token: "fixture-csrf", expires_at: "2099-01-01T00:00:00Z" } };
      if (path === "/api/v1/identity/characters") data = { characters: [{ id: "101", name: "Pilot", status: "active", is_main: true }] };
      if (path === "/api/v1/eve/status") data = { configured: true };
      if (path === "/api/v1/access/me") data = { administrator: false, site_roles: [], characters: [] };
      if (path === "/api/v1/community/profile") data = { version: "1", complete: true, qq: { value: "123456789", version: "1", confirmation: status === "bound" ? "confirmed" : "pending" }, kook: { value: "Pilot", version: "1", confirmation: "pending" } };
      if (path === "/api/v1/community/qq/group/options") data = { items: [group] };
      if (path === "/api/v1/community/qq/group/application") {
        if (request.method() === "POST") {
          expect(request.postDataJSON()).toEqual({ group_openid: group.group_openid });
          status = "pending";
          httpStatus = 201;
          data = { ...application, status, code: "QQCODE89" };
        } else {
          data = status ? { ...application, status } : null;
        }
      }
      return route.fulfill({ status: httpStatus, json: { data } });
    });
    await mockCharacterSync(page);
    await page.goto("/account");
    await page.getByRole("button", { name: en ? "Generate group admission code" : "生成入群绑定码" }).click();
    await expect(page.getByText("QQCODE89", { exact: true })).toBeVisible();
    await expect(page.getByRole("link", { name: en ? "Open QQ to request to join" : "打开 QQ 申请入群" })).toHaveAttribute("href", group.join_url);
    status = "waiting_membership";
    await expect(page.getByText(en ? "Request received. Approval is automatic once SEAT syncs your corporation membership." : "已收到入群申请，加入军团并同步后自动通过")).toBeVisible({ timeout: 10000 });
    await expect(page.getByText("QQCODE89", { exact: true })).toHaveCount(0);
    status = "bound";
    await expect(page.getByText(en ? "Group joined and QQ linked" : "已入群，QQ 已绑定")).toBeVisible({ timeout: 10000 });
  });
}
