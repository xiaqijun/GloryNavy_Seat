import { test, expect } from "@playwright/test";
import { mockCharacterSync } from "./sync-fixture";

for (const locale of ["zh-CN", "en"]) {
  test(`NapCat admin configuration and mention ${locale}`, async ({ page }) => {
    const en = locale === "en";
    await page.addInitScript((value) => localStorage.setItem("glorynavy.locale", value), locale);
    await page.route("https://images.evetech.net/**", r => r.abort());
    let settings = { enabled: true, api_base: "http://127.0.0.1:3000", bot_qq: "999999", token_configured: true, secret_configured: true, revision: "1", groups: [] as {group_id:string;name:string}[] };
    let fails = true, sends = 0;
    await page.route("**/api/v1/**", async r => {
      const path = new URL(r.request().url()).pathname;
      let data: unknown = {};
      if (path === "/api/v1/modules") data = ["system", "identity", "eve", "access", "community"].map(id => ({ id, api_version: 1, version: "0.1.0" }));
      if (path === "/api/v1/identity/session") data = { authenticated: true, session: { user_id: "admin", character: { id: "101", name: "Pilot" }, csrf_token: "test-csrf", expires_at: "2099-01-01T00:00:00Z" } };
      if (path === "/api/v1/identity/characters") data = { characters: [{ id: "101", name: "Pilot", status: "active", is_main: true }] };
      if (path === "/api/v1/eve/status") data = { configured: true };
      if (path === "/api/v1/access/me") data = { administrator: true, site_roles: [], characters: [] };
      if (path === "/api/v1/community/profile") data = { version: "1", complete: true, qq: { value: "123456789", version: "1", confirmation: "confirmed" }, kook: { value: "Pilot", version: "1", confirmation: "pending" } };
      if (path === "/api/v1/community/qq/onebot/settings") {
        if (r.request().method() === "PUT") {
          expect(r.request().headers()["x-csrf-token"]).toBe("test-csrf");
          const input = r.request().postDataJSON();
          expect(input.token).toBe(""); expect(input.secret).toBe("");
          expect(input.revision).toBe(settings.revision);
          settings = { ...settings, ...input, revision: String(Number(settings.revision) + 1) };
        }
        data = settings;
      }
      if (path === "/api/v1/community/qq/onebot/status") {
        if (fails) return r.fulfill({status:503,json:{error:{message:"Connection unavailable"}}});
        data = { online: true, bot_qq: "999999", nickname: "Test bot", groups: [{group_id:"888888",name:"荣耀海军测试群 VeryLongGroupNameForResponsiveLayout"}] };
      }
      if (path === "/api/v1/community/qq/onebot/test") {
        expect(r.request().headers()["x-csrf-token"]).toBe("test-csrf");
        expect(r.request().postDataJSON()).toEqual({group_id:"888888"}); sends++; data={sent:true};
      }
      await r.fulfill({json:{data}});
    });
    await mockCharacterSync(page);
    await page.goto("/community");
    const connect = page.getByRole("button", { name: en ? "Connect and load groups" : "连接并获取群", exact: true });
    await connect.click();
    await expect(page.getByRole("alert")).toHaveText("Connection unavailable");
    fails = false; await connect.click();
    await expect(page.getByRole("status")).toContainText("Test bot");
    await page.locator(".onebot-group-choice input").check();
    const mention = page.getByRole("button", { name: /测试 @我|Test a mention of me/ });
    await expect(mention).toBeDisabled();
    await page.getByRole("button", { name: en ? "Save" : "保存", exact:true }).first().click();
    await expect(mention).toBeEnabled();
    await mention.click(); await expect.poll(() => sends).toBe(1);
    await expect(page.getByRole("status")).toContainText(en ? "Test message submitted" : "测试消息已提交");
    await expect(page.locator('input[type="password"]').first()).toHaveValue("");
    for (const width of [1280,375,320]) {
      await page.setViewportSize({width,height:950});
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      await page.locator(".community-admin-page").screenshot({path:`../.local/qq-napcat/admin-${locale}-${width}.png`});
    }
  });
}
