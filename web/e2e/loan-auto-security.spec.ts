import { test, expect } from "@playwright/test";
import { english } from "../src/lib/locales/en";

for (const language of ["zh-CN", "en"]) {
  test(`only excess needs security and guarantee amounts update automatically (${language})`, async ({ page }, info) => {
    const tr = (s: string) => language === "en" ? english[s] ?? s : s;
    const lenderName = "繁荣海军统一资金池 Glory Navy Shared Lending Pool";
    const officerName = "Loan Officer With A Very Long Character Name";
    const loan = { id: "1", public_id: "LN-20261016-LENDER-DETAILS", pool_id: "1", pool_name: lenderName, lender_kind: "corporation", borrower_account_id: "borrower", borrower_character_id: "111", principal_minor: 1200000000000, interest_minor: 10000000000, interest_rate_bps: 1200, total_due_minor: 1210000000000, installment_count: 1, interval_days: 30, state: "active", version: 1, created_at: "2026-10-16T00:00:00Z" };
    const pool = { id: "1", name: lenderName, is_shared: true, lender_kind: "corporation", state: "open", custodian_character_id: "333", custodian_character_name: officerName, config: {}, version: 1 };
    let mode = "within", submitted = false, attempts = 0;
    const guarantees: unknown[] = [];
    loan.state = "submitted";
    const security = () => ({ credit_ready: true, available_credit_minor: mode === "within" ? loan.principal_minor : loan.principal_minor - (mode === "changed" ? 150000000000 : 200000000000), required_minor: mode === "within" ? 0 : mode === "changed" ? 150000000000 : 200000000000, effective_minor: 0, pending_minor: submitted ? 150000000000 : 0, unallocated_minor: mode === "within" || submitted ? 0 : mode === "changed" ? 150000000000 : 200000000000, legacy_pending: false });
    await page.addInitScript(l => localStorage.setItem("glorynavy.locale", l), language);
    await page.route("https://images.evetech.net/**", r => r.abort());
    await page.route("**/api/v1/**", async r => {
      const path = new URL(r.request().url()).pathname;
      let data: unknown = {};
      if (path.endsWith("/modules")) data = ["system", "identity", "eve", "access", "loan"].map(id => ({ id, version: "0.1.0", api_version: 1 }));
      else if (path.endsWith("/identity/session")) data = { authenticated: true, session: { user_id: "borrower", character: { id: "111", name: "Loan Pilot" }, csrf_token: "csrf", expires_at: "2099-01-01T00:00:00Z" } };
      else if (path.endsWith("/access/me")) data = { administrator: false, can_manage: false, can_manage_sync: false, site_roles: [], characters: [] };
      else if (path.endsWith("/identity/characters")) data = { characters: [{ id: "111", name: "Loan Pilot", status: "active", is_main: true }] };
      else if (path.endsWith("/community/profile")) data = { version: "1", complete: true, qq: { value: "10000", version: "1", confirmation: "confirmed" }, kook: { value: "Pilot", version: "1", confirmation: "confirmed" } };
      else if (path.endsWith("/loan/context")) data = { administrator: false, can_manage_pool: false, pools: [pool], credit: { score: 80, total_limit_minor: 1000000000000, state: "active" } };
      else if (path.endsWith("/loan/cases")) data = { items: [loan] };
      else if (path.endsWith("/cases/1")) data = { security: security(), case: loan, guarantees, collateral: [], collateral_requests: [], installments: [] };
      else if (path.endsWith("/guarantees/candidates")) data = { items: [{ id: "222", name: "Guarantor Pilot" }] };
      else if (path.endsWith("/cases/1/guarantees")) {
        attempts++;
        expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
        expect(r.request().postDataJSON().amount_minor).toBe(attempts === 1 ? 200000000000 : 150000000000);
        if (attempts === 1) { mode = "changed"; return r.fulfill({ status: 409, json: { error: { message: "Credit changed; retry" } } }); }
        submitted = true;
        data = { id: "1", guarantor_account_id: "guarantor", guarantor_character_id: "222", guarantor_character_name: "Guarantor Pilot", amount_minor: 150000000000, state: "invited", version: 1 };
        guarantees.push(data);
      }
      else if (path.includes("/loan/")) data = { items: [] };
      return r.fulfill({ json: { data } });
    });
    await page.goto("/loans/operations?view=business");
    await page.getByRole("button", { name: tr("查看详情"), exact: true }).click();
    let dialog = page.getByRole("dialog");
    await expect(dialog).toContainText(tr("借款在可用个人额度内，无需担保或抵押。"));
    await expect(dialog.getByRole("button", { name: tr("找人担保"), exact: true })).toHaveCount(0);
    await expect(dialog.getByRole("button", { name: tr("物品抵押"), exact: true })).toHaveCount(0);
    mode = "excess";
    await page.reload();
    await page.getByRole("button", { name: tr("查看详情"), exact: true }).click();
    dialog = page.getByRole("dialog");
    await expect(dialog).toContainText("2,000,000,000 ISK");
    await page.screenshot({ path: info.outputPath("excess-detail.png"), fullPage: true });
    await dialog.getByRole("button", { name: tr("找人担保"), exact: true }).click();
    const amount = dialog.getByLabel(tr("担保金额（ISK）"));
    await expect(amount).toHaveValue("2000000000");
    await expect(amount).toHaveAttribute("readonly", "");
    const character = dialog.getByRole("combobox");
    await expect(character).toHaveValue("222");
    await dialog.getByRole("button", { name: tr("发送邀请"), exact: true }).click();
    await expect(dialog.getByRole("alert")).toHaveText("Credit changed; retry");
    await expect(amount).toHaveValue("1500000000");
    await page.screenshot({ path: info.outputPath("automatic-guarantee.png"), fullPage: true });
    await dialog.getByRole("button", { name: tr("发送邀请"), exact: true }).click();
    await expect(dialog).toContainText(tr("保障正在办理，确认或审核通过后才生效。"));
    await expect(dialog.getByRole("button", { name: tr("物品抵押"), exact: true })).toHaveCount(0);
    await expect.poll(() => dialog.evaluate(e => e.scrollWidth <= e.clientWidth)).toBe(true);
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.screenshot({ path: info.outputPath("pending-guarantee.png"), fullPage: true });
  });
}
