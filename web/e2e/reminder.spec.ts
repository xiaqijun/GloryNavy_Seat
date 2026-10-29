import { test, expect, type Page } from "@playwright/test";
import { english } from "../src/lib/locales/en";
async function fixture(page: Page, admin = true, ready = true) {
  const available_events = [{"id": "approval_pending", "name": "待审批提醒"}, {"id": "feedback_update", "name": "反馈回复与进度"}, {"id": "feedback_pending", "name": "待处理反馈"}, {"id": "collateral_pending", "name": "待交付抵押"}, {"id": "contribution_result", "name": "出借到账"}, {"id": "welfare_result", "name": "福利申请进度"}, {"id": "exchange_result", "name": "兑换进度"}, {"id": "loan_result", "name": "贷款进度"}, {"id": "contract_pending", "name": "待领取合同"}, {"id": "guarantee_pending", "name": "待处理担保"}, {"id": "loan_funding", "name": "待放款提醒"}, {"id": "loan_due", "name": "贷款即将到期"}, {"id": "loan_overdue", "name": "贷款逾期"}, {"id": "pap_requirement", "name": "PAP 未达标"}, {"id": "pap_change", "name": "PAP 变动"}, {"id": "coin_change", "name": "果壳币变动"}, {"id": "authorization_expired", "name": "角色授权失效"}, {"id": "structure_alert", "name": "建筑燃料与拆除"}, {"id": "sync_anomaly", "name": "数据同步异常"}, {"id": "sentry_alert", "name": "预警服务异常"}];
  const state = { config: { events: ["approval_pending"], available_events, enabled: false, sender_id: "101", sender_name: "荣耀海军邮件专员 Mail Sender", revision: "1", status: ready ? "ready" : "authorization_required", characters: [{ id: "101", name: "荣耀海军邮件专员 Mail Sender", ready }] }, fail: false, saved: 0, retried: 0, auth: 0 };
  await page.route("https://images.evetech.net/**", r => r.abort());
  await page.route("**/api/v1/**", async r => {
    const path = new URL(r.request().url()).pathname;
    let data: unknown = {};
    if (path === "/api/v1/modules") data = ["system", "identity", "eve", "access", "approval", "reminder"].map(id => ({ id, api_version: 1, version: "0.1.0" }));
    if (path === "/api/v1/identity/session") data = { authenticated: true, session: { user_id: "admin", character: { id: "101", name: "Pilot" }, csrf_token: "test-csrf", expires_at: "2099-01-01T00:00:00Z" } };
    if (path === "/api/v1/identity/characters") data = { characters: [{ id: "101", name: "Pilot", status: "active", is_main: true }] };
    if (path === "/api/v1/eve/status") data = { configured: true };
    if (path === "/api/v1/access/me") data = { administrator: admin, site_roles: [], characters: [] };
    if (path === "/api/v1/reminder/email/settings") data = { enabled: false, host: "", port: 465, implicit_tls: true, username: "", sender: "", secret_configured: false, revision: "1", events: [], available_events: [] };
    if (path === "/api/v1/reminder/email/deliveries") data = [];
    if (path === "/api/v1/reminder/settings") {
      expect(admin).toBe(true);
      if (r.request().method() === "PUT") {
        expect(r.request().headers()["x-csrf-token"]).toBe("test-csrf");
        if (state.fail) return r.fulfill({ status: 409, json: { error: { message: "Version changed" } } });
        expect(r.request().postDataJSON().revision).toBe(state.config.revision);
        state.config = { ...state.config, ...r.request().postDataJSON(), revision: String(Number(state.config.revision) + 1) }; state.saved++;
      }
      data = state.config;
    }
    if (path === "/api/v1/reminder/deliveries") data = ["sent", "failed", "unknown"].map((s, i) => ({ id: `id-${i}`, channel: "eve_mail", event: "approval_pending", sender_id: "101", recipient_id: "202", recipient_name: "千珏 VeryLongCharacterName", notice_count: 9, state: s === "failed" && state.retried ? "retry_requested" : s, reason: s === "unknown" ? "confirmation_required" : "", message_id: "999", created_at: "2026-10-17T12:00:00Z" }));
    if (path.endsWith("/retry")) { expect(r.request().headers()["x-csrf-token"]).toBe("test-csrf"); state.retried++; data = { queued: true }; }
    if (path.endsWith("/reauthorize")) { expect(new URL(r.request().url()).searchParams.get("purpose")).toBe("reminder-mail"); state.auth++; return r.fulfill({ status: 503, json: { error: { message: "SSO unavailable" } } }); }
    await r.fulfill({ json: { data } });
  });
  return state;
}
for (const lang of ["zh-CN", "en"]) {
  test(`reminder callback recovery messages ${lang}`, async ({ page }) => {
    const tr = (s: string) => lang === "en" ? english[s] ?? s : s;
    await page.addInitScript(l => localStorage.setItem("glorynavy.locale", l), lang);
    await fixture(page, true, false);
    for (const [code, text] of [["session_expired", "登录已失效，请重新登录后授权发信"], ["authorization_required", "缺少发信权限，请确认 EVE 应用权限后重新授权"], ["forbidden", "当前账号无权授权发信"]]) {
      await page.goto(`/reminders?error=${code}`);
      const game = page.locator('[data-slot="card"]').filter({ has: page.getByRole("heading", { name: tr("游戏内邮件") }) });
      await expect(page.getByRole("alert")).toContainText(tr(text));
      await expect(game.getByRole("checkbox", { name: tr("启用推送") })).toBeDisabled();
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
      await page.getByRole("button", { name: tr("关闭"), exact: true }).click();
      await expect(page.getByRole("alert")).toHaveCount(0);
    }
  });
  test(`reminder settings, safe retry and compact layout ${lang}`, async ({ page }, info) => {
    const tr = (s: string) => lang === "en" ? english[s] ?? s : s;
    await page.addInitScript(l => localStorage.setItem("glorynavy.locale", l), lang);
    const state = await fixture(page);
    await page.goto("/reminders");
    const game = page.locator('[data-slot="card"]').filter({ has: page.getByRole("heading", { name: tr("游戏内邮件") }) });
    const toggle = game.getByRole("checkbox", { name: tr("启用推送") });
    await expect(toggle).not.toBeChecked();
    await game.getByRole("button", { name: tr("全选"), exact: true }).click();
    await expect(page.getByRole("checkbox", { name: tr("待领取合同"), exact: true })).toBeChecked();
    await toggle.check(); state.fail = true;
    await game.getByRole("button", { name: tr("保存"), exact: true }).click();
    await expect(page.getByRole("alert")).toContainText("Version changed"); await expect(toggle).toBeChecked();
    state.fail = false; await game.getByRole("button", { name: tr("保存"), exact: true }).click();
    await expect.poll(() => state.saved).toBe(1); expect(state.config.events.length).toBe(20); await expect(toggle).toBeChecked();
    await expect(page.locator("tbody tr")).toHaveCount(3);
    await expect(page.locator("tbody tr").nth(2).getByRole("button")).toHaveCount(0);
    await page.locator("tbody tr").nth(1).getByRole("button").click();
    await expect.poll(() => state.retried).toBe(1);
    await expect(page.locator("tbody tr").nth(1)).toContainText(tr("等待重试"));
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    await page.screenshot({ path: info.outputPath(`reminder-${lang}.png`), fullPage: true });
  });
  test(`reminder scope authorization and nonadmin ${lang}`, async ({ page }) => {
    const tr = (s: string) => lang === "en" ? english[s] ?? s : s;
    await page.addInitScript(l => localStorage.setItem("glorynavy.locale", l), lang);
    const state = await fixture(page, true, false); await page.goto("/reminders?error=wrong_character");
    const game = page.locator('[data-slot="card"]').filter({ has: page.getByRole("heading", { name: tr("游戏内邮件") }) });
    await expect(game.getByRole("checkbox", { name: tr("启用推送") })).toBeDisabled();
    await expect(page.getByRole("alert")).toContainText(tr("授权角色不符，请在 EVE 页面选择 {0}").replace("{0}", state.config.sender_name));
    await page.getByRole("button", { name: tr("授权发信"), exact: true }).click();
    await expect.poll(() => state.auth).toBe(1); await expect(page.getByText("SSO unavailable")).toBeVisible();
    await page.unrouteAll(); await fixture(page, false); await page.reload();
    await expect(page.getByRole("checkbox", { name: tr("启用推送") })).toHaveCount(0);
    await expect(page.getByRole("button", { name: tr("授权发信"), exact: true })).toHaveCount(0);
  });
}
