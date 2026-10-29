import { test, expect, type Page } from "@playwright/test";
import { english } from "../src/lib/locales/en";

async function fixture(page: Page) {
  const state = { value: { enabled: true, group_openid: "official-group", corporation_id: "42", corporation_name: "Glory Navy", revision: "2", bot_ready: true, bot_changed: false }, saves: 0, conflict: false };
  await page.route("https://images.evetech.net/**", route => route.abort());
  await page.route("**/api/v1/**", async route => {
    const path = new URL(route.request().url()).pathname;
    let data: unknown = {};
    if (path === "/api/v1/modules") data = ["system", "identity", "eve", "access", "community", "structures", "approval", "reminder"].map(id => ({ id, api_version: 1, version: "0.1.0" }));
    if (path === "/api/v1/identity/session") data = { authenticated: true, session: { user_id: "admin", character: { id: "101", name: "Pilot" }, csrf_token: "csrf", expires_at: "2099-01-01T00:00:00Z" } };
    if (path === "/api/v1/identity/characters") data = { characters: [{ id: "101", name: "Pilot", status: "active", is_main: true }] };
    if (path === "/api/v1/eve/status") data = { configured: true };
    if (path === "/api/v1/access/me") data = { administrator: true, site_roles: [], characters: [] };
    if (path === "/api/v1/access/scope-options") data = { corporations: [{ id: "42", name: "Glory Navy" }, { id: "43", name: "Second Corporation" }], alliances: [] };
    if (path === "/api/v1/reminder/settings") data = { enabled: false, sender_id: "0", sender_name: "", revision: "1", status: "not_configured", characters: [], events: [], available_events: [] };
    if (path === "/api/v1/reminder/deliveries") data = [];
    if (path === "/api/v1/reminder/email/settings") data = { enabled: false, host: "", port: 465, implicit_tls: true, username: "", sender: "", secret_configured: false, revision: "1", events: [], available_events: [] };
    if (path === "/api/v1/reminder/email/deliveries") data = [];
    if (path === "/api/v1/community/qq/structure/settings") {
      if (route.request().method() === "PUT") {
        expect(route.request().headers()["x-csrf-token"]).toBe("csrf");
        if (state.conflict) return route.fulfill({ status: 409, json: { error: { message: "Version changed" } } });
        const value = route.request().postDataJSON();
        expect(value.revision).toBe(state.value.revision);
        state.value = { ...state.value, ...value, revision: String(Number(value.revision) + 1) };
        state.saves++;
      }
      data = state.value;
    }
    if (path === "/api/v1/community/qq/structure/deliveries") data = [{ id: "1", body: "Station · fuel alert", kind: "fuel", status: "unknown", error_code: 0, created_at: "2026-10-29T06:00:00Z" }];
    await route.fulfill({ json: { data } });
  });
  return state;
}

for (const lang of ["zh-CN", "en"]) {
  test(`QQ structure settings ${lang}`, async ({ page }, info) => {
    const tr = (s: string) => lang === "en" ? english[s] ?? s : s;
    await page.addInitScript(l => localStorage.setItem("glorynavy.locale", l), lang);
    const state = await fixture(page);
    await page.goto("/reminders");
    const card = page.locator('[data-slot="card"]').filter({ has: page.getByRole("heading", { name: tr("QQ群建筑提醒") }) });
    await expect(card.getByText("Glory Navy")).toBeVisible();
    await expect(card.getByText(tr("结果待确认"))).toBeVisible();
    const input = card.getByLabel(tr("群 OpenID"));
    await input.fill("123456");
    await expect(card.getByRole("alert")).toContainText(tr("请填写官方群 OpenID，不能使用数字群号"));
    await expect(card.getByRole("button", { name: tr("保存"), exact: true })).toBeDisabled();
    await input.fill("another-official-group");
    state.conflict = true;
    await card.getByRole("button", { name: tr("保存"), exact: true }).click();
    await expect(card.getByRole("alert")).toContainText("Version changed");
    await expect(input).toHaveValue("another-official-group");
    state.conflict = false;
    await card.getByRole("combobox", { name: tr("军团") }).click();
    await page.getByRole("option", { name: "Second Corporation" }).click();
    await card.getByRole("button", { name: tr("保存"), exact: true }).click();
    await expect.poll(() => state.saves).toBe(1);
    expect(state.value.corporation_id).toBe("43");
    await expect(card.getByText(tr("建筑提醒配置已保存"))).toBeVisible();
    await card.getByRole("checkbox", { name: tr("启用推送") }).uncheck();
    await card.getByRole("button", { name: tr("保存"), exact: true }).click();
    await expect.poll(() => state.saves).toBe(2);
    expect(state.value.enabled).toBe(false);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    if (info.project.name !== "desktop") expect(await card.locator(".qq-structure-records").evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true);
    await page.screenshot({ path: info.outputPath(`qq-structure-${lang}.png`), fullPage: true });
  });
}
