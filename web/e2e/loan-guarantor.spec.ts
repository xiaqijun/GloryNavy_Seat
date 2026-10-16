import { test, expect } from "@playwright/test";
import { english } from "../src/lib/locales/en";

for (const language of ["zh-CN", "en"]) {
  test(`guarantor identity, amount and missing-name fallbacks (${language})`, async ({ page }, info) => {
    const tr = (s: string) => language === "en" ? english[s] ?? s : s;
    const lenderName = "繁荣海军统一资金池 Glory Navy Shared Lending Pool";
    const officerName = "Loan Officer With A Very Long Character Name";
    const loan = { id: "1", public_id: "LN-20261016-LENDER-DETAILS", pool_id: "1", pool_name: lenderName, lender_kind: "corporation", borrower_account_id: "borrower", borrower_character_id: "111", principal_minor: 1200000000000, interest_minor: 10000000000, interest_rate_bps: 1200, total_due_minor: 1210000000000, installment_count: 1, interval_days: 30, state: "active", version: 1, created_at: "2026-10-16T00:00:00Z" };
    const pool = { id: "1", name: lenderName, is_shared: true, lender_kind: "corporation", state: "open", custodian_character_id: "333", custodian_character_name: officerName, config: {}, version: 1 };
    const guarantees = [
      { id: "1", guarantor_character_id: "222", guarantor_character_name: "担保人甲 Guarantor Alpha", guarantor_account_id: "account-a", amount_minor: 200000000000, state: "accepted", version: 1 },
      { id: "2", guarantor_character_id: "333", guarantor_character_name: "GuarantorWithAnExtremelyLongUnbrokenCharacterName", guarantor_account_id: "account-b", amount_minor: 50000000000, state: "invited", version: 1 },
      { id: "3", guarantor_character_id: "444", guarantor_account_id: "account-c", amount_minor: 10000000000, state: "released", version: 1 },
      { id: "4", guarantor_character_id: "0", guarantor_account_id: "00000000-0000-4000-8000-000000000004", amount_minor: 100000000, state: "rejected", version: 1 },
    ];
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
      else if (path.endsWith("/cases/1")) data = { case: loan, guarantees, collateral: [], collateral_requests: [], installments: [] };
      else if (path.includes("/loan/")) data = { items: [] };
      return r.fulfill({ json: { data } });
    });
    await page.goto("/loans/operations?view=business");
    await page.getByRole("button", { name: tr("查看详情"), exact: true }).click();
    const dialog = page.getByRole("dialog");
    const section = dialog.locator(".loan-detail-section").filter({ has: page.getByRole("heading", { name: tr("他人担保"), exact: true }) });
    const records = section.getByRole("listitem");
    await expect(records).toHaveCount(4);
    const identities = [guarantees[0].guarantor_character_name!, guarantees[1].guarantor_character_name!, tr("角色 #{0}").replace("{0}", "444"), tr("担保账号 {0}").replace("{0}", guarantees[3].guarantor_account_id)];
    const states = ["已接受", "待确认", "已释放", "已拒绝"];
    for (let i = 0; i < guarantees.length; i++) {
      await expect(records.nth(i)).toContainText(identities[i]);
      await expect(records.nth(i)).toContainText(tr(states[i]));
      await expect(records.nth(i).locator(".loan-guarantee-amount")).toHaveText(`${new Intl.NumberFormat(language).format(guarantees[i].amount_minor / 100)} ISK`);
    }
    await records.first().scrollIntoViewIfNeeded();
    await expect.poll(() => dialog.evaluate(e => e.scrollWidth <= e.clientWidth)).toBe(true);
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.screenshot({ path: info.outputPath("guarantors.png"), fullPage: true });
    await records.last().scrollIntoViewIfNeeded();
    await page.screenshot({ path: info.outputPath("fallbacks.png"), fullPage: true });
  });
}
