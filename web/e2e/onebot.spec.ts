import { test, expect } from "@playwright/test";
import { mockCharacterSync } from "./sync-fixture";

for (const locale of ["zh-CN", "en"]) {
  for (const admin of [true, false]) {
    test(`Official QQ configuration ${locale} administrator=${admin}`, async ({ page }, info) => {
      const en = locale === "en";
      await page.addInitScript(value => localStorage.setItem("glorynavy.locale", value), locale);
      await page.route("https://images.evetech.net/**", r => r.abort());
      let settings = { enabled: false, app_id: "test-app", api_base: "https://api.bot.qq.com", secret_configured: true, initialized: true, revision: "1" };
      let unavailable = true, conflict = true, saves = 0, reads = 0, onebot = 0;
      let groups: { group_openid: string; label: string; enabled: boolean }[] = [];
      await page.route("**/api/v1/**", async r => {
        const path = new URL(r.request().url()).pathname;
        let data: unknown = {};
        if (path.includes("/onebot/")) { onebot++; throw new Error("Retired transport requested"); }
        if (path === "/api/v1/modules") data = ["system", "identity", "eve", "access", "community"].map(id => ({ id, api_version: 1, version: "0.1.0" }));
        if (path === "/api/v1/identity/session") data = { authenticated: true, session: { user_id: "admin", character: { id: "101", name: "Pilot" }, csrf_token: "test-csrf", expires_at: "2099-01-01T00:00:00Z" } };
        if (path === "/api/v1/identity/characters") data = { characters: [{ id: "101", name: "Pilot", status: "active", is_main: true }] };
        if (path === "/api/v1/eve/status") data = { configured: true };
        if (path === "/api/v1/access/me") data = { administrator: admin, site_roles: [], characters: [] };
        if (path === "/api/v1/community/profile") data = { version: "1", complete: true, qq: { value: "123456789", version: "1", confirmation: "pending" }, kook: { value: "Pilot", version: "1", confirmation: "pending" } };
        if (path === "/api/v1/community/qq/bot/settings") {
          reads++;
          expect(admin).toBe(true);
          if (r.request().method() === "PUT") {
            const input = r.request().postDataJSON();
            expect(r.request().headers()["x-csrf-token"]).toBe("test-csrf");
            expect(input.revision).toBe(settings.revision);
            if (conflict) {
              conflict = false; settings = { ...settings, revision: "2" };
              return r.fulfill({ status: 409, json: { error: { message: "Settings changed" } } });
            }
            expect(input.secret).toBe("");
            settings = { ...settings, enabled: input.enabled, revision: String(Number(settings.revision) + 1) };
            saves++;
          } else if (unavailable) {
            return r.fulfill({ status: 503, json: { error: { message: "Fixture unavailable" } } });
          }
          data = settings;
        }
        if (path === "/api/v1/community/qq/group/settings") {
          expect(admin).toBe(true);
          if (r.request().method() === "PUT") {
            expect(r.request().headers()["x-csrf-token"]).toBe("test-csrf");
            groups = r.request().postDataJSON().items;
          }
          data = { items: groups, initialized: true };
        }
        await r.fulfill({ json: { data } });
      });
      await mockCharacterSync(page);
      await page.goto("/community");
      if (!admin) {
        await expect(page.getByRole("heading", {level: 1})).toHaveText(en ? "Page not found" : "页面不存在");
        expect(reads).toBe(0); expect(onebot).toBe(0);
        return;
      }
      await expect(page.getByRole("alert").filter({ hasText: "Fixture unavailable" })).toBeVisible();
      unavailable = false;
      await page.getByRole("button", { name: en ? "Retry" : "重试", exact: true }).click();
      const card = page.locator(".qq-bot-settings-card");
      await expect(card.getByRole("heading")).toHaveText(en ? "Official QQ Bot" : "官方 QQ Bot");
      await expect(card.locator(".status-chip").first()).toHaveText(en ? "Bot disabled" : "机器人已停用");
      await expect(card.locator("#qq-bot-callback")).toHaveValue(/\/api\/v1\/community\/qq\/official\/webhook$/);
      const enabled = card.getByRole("checkbox");
      const save = card.getByRole("button", { name: en ? "Save QQ settings" : "保存 QQ 配置", exact: true });
      await enabled.check();
      await card.locator("#qq-bot-secret").fill("new-test-secret");
      await save.click();
      await expect(card.getByRole("alert")).toHaveText("Settings changed");
      await expect(card.locator("#qq-bot-secret")).toHaveValue("new-test-secret");
      await card.getByRole("button", { name: en ? "Reload" : "重新读取", exact: true }).click();
      await expect(card.locator("#qq-bot-secret")).toHaveValue("");
      await enabled.check();
      await save.click();
      await expect.poll(() => saves).toBe(1);
      await expect(card.locator(".status-chip").first()).toHaveText(en ? "Bot enabled" : "机器人已启用");
      await expect(enabled).toBeChecked();
      await enabled.uncheck();
      await save.click();
      await expect.poll(() => saves).toBe(2);
      await expect(card.locator(".status-chip").first()).toHaveText(en ? "Bot disabled" : "机器人已停用");
      expect(settings.secret_configured).toBe(true);
      const groupCard = page.locator(".qq-group-settings-card");
      await groupCard.getByRole("button", { name: en ? "Add group" : "添加群", exact: true }).click();
      await groupCard.locator("#qq-group-openid-0").fill("test-group-openid");
      await groupCard.locator("#qq-group-label-0").fill("荣耀海军测试群 VeryLongGroupNameForResponsiveLayout");
      await groupCard.getByRole("button", { name: en ? "Save settings" : "保存配置", exact: true }).click();
      await expect.poll(() => groups.length).toBe(1);
      expect(onebot).toBe(0);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      if (page.viewportSize()!.width <= 520) {
        for (const id of ["#qq-group-openid-0", "#qq-group-label-0"]) {
          expect((await groupCard.locator(id).boundingBox())!.width).toBeGreaterThan(180);
        }
      }
      await page.evaluate(() => window.scrollTo(0, 0));
      await page.screenshot({ path: info.outputPath(`official-top-${locale}.png`) });
      await groupCard.scrollIntoViewIfNeeded();
      await page.screenshot({ path: info.outputPath(`official-groups-${locale}.png`) });
      await page.locator(".community-admin-page").screenshot({ path: info.outputPath(`official-${locale}.png`) });
    });
  }
}
