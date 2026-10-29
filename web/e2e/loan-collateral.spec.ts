import { test, expect } from "@playwright/test";
import { english } from "../src/lib/locales/en";

for (const language of ["zh-CN", "en"]) {
  test(`select personal assets, deliver collateral and inspect details (${language})`, async ({ page }, info) => {
    const tr = (s: string) => language === "en" ? english[s] ?? s : s;
    const loan = { id: "1", public_id: "LN-20261016-LONG-APPLICATION-ID", pool_id: "1", borrower_account_id: "borrower", borrower_character_id: "111", principal_minor: 123456789012345, interest_minor: 123456789, interest_rate_bps: 1200, total_due_minor: 123456912469134, installment_count: 1, interval_days: 365, state: "submitted", version: 1, created_at: "2026-10-16T00:00:00Z" };
    const assets = { character_id: "111", observed_at: "2026-10-16T00:00:00Z", items: [
      { item_id: "501", type_id: "34", name: "Tritanium", quantity: "10", available: "10", location_id: "6001", items: [{ item_id: "501", type_id: "34", quantity: "10", name: "Tritanium", included: true }] },
      { item_id: "502", type_id: "35", name: "Pyerite", quantity: "1", available: "1", location_id: "6002", items: [{ item_id: "502", type_id: "35", quantity: "1", name: "Pyerite", included: true }] },
    ] };
    const pending: any[] = [], collateral: any[] = [];
    const security = () => ({ credit_ready: true, available_credit_minor: loan.principal_minor - 200000, required_minor: 200000, effective_minor: 0, pending_minor: pending.length ? 200000 : 0, unallocated_minor: pending.length ? 0 : 200000, legacy_pending: false });
    let contractReads = 0, writes = 0;
    await page.addInitScript(l => localStorage.setItem("glorynavy.locale", l), language);
    await page.route("https://images.evetech.net/**", r => r.abort());
    await page.route("**/api/v1/**", async r => {
      const path = new URL(r.request().url()).pathname;
      let data: unknown = {};
      if (path.endsWith("/modules")) data = ["system", "identity", "eve", "access", "loan"].map(id => ({ id, version: "0.1.0", api_version: 1 }));
      else if (path.endsWith("/identity/session")) data = { authenticated: true, session: { user_id: "borrower", character: { id: "111", name: "Loan Pilot" }, csrf_token: "csrf", expires_at: "2099-01-01T00:00:00Z" } };
      else if (path.endsWith("/access/me")) data = { administrator: true, can_manage: true, can_manage_sync: false, site_roles: [], characters: [] };
      else if (path.endsWith("/identity/characters")) data = { characters: [{ id: "111", name: "Loan Pilot With A Long Character Name", status: "active", is_main: true }] };
      else if (path.endsWith("/community/profile")) data = { version: "1", complete: true, qq: { value: "10000", version: "1", confirmation: "confirmed" }, kook: { value: "Pilot", version: "1", confirmation: "confirmed" } };
      else if (path.endsWith("/loan/context")) data = { administrator: true, can_manage_pool: true, loan_officers: [{id:"333",name:"Officer Main",is_main:true},{id:"334",name:"Officer Alt"}], pools: [{ id: "1", name: "Shared pool", is_shared: true, lender_kind: "corporation", state: "open", custodian_character_id: "333", custodian_character_name: "Loan Custodian", config: {}, version: 1 }], credit: { score: 80, total_limit_minor: 100000000, state: "active" }, pool_summary: { pending_minor: 0, funded_minor: 0, cash_minor: 100000000, reserved_minor: 0, available_minor: 100000000 } };
      else if (path.endsWith("/loan/cases")) data = { items: [loan] };
      else if (path.endsWith("/cases/1")) data = { security: security(), case: loan, guarantees: [], collateral, collateral_requests: pending, installments: [] };
      else if (path.endsWith("/collateral-assets")) data = assets;
      else if (path.endsWith("/collateral-quote")) {
        const body = r.request().postDataJSON();
        const amount = body.items.reduce((sum: number, i: { quantity: string }) => sum + Number(i.quantity) * 125000, 0);
        const target = body.allocation_minor || 200000;
        data = { security: security(), valuation_minor: amount, covered_minor: Math.floor(amount * 0.8), required_minor: 200000, target_minor: target, shortfall_minor: Math.max(0, target - Math.floor(amount * 0.8)), can_submit: amount * 0.8 >= target };
      }
      else if (path.endsWith("/collateral-requests")) {
        expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
        expect(r.request().postDataJSON()).toEqual({ character_id: "111", recipient_character_id: "334", coverage_minor: 200000, allocation_minor: 200000, items: [{ item_id: "501", quantity: "3" }] });
        writes++;
        if (writes === 1) return r.fulfill({ status: 409, json: { error: { message: "Assets changed; retry" } } });
        data = { coverage_minor: 200000, valuation_minor: 375000, id: "1", public_id: "LNC-TEST-DELIVERY", case_id: "1", source_character_id: "111", recipient_character_id: "334", items: [{ type_id: "34", quantity: "3", name: "Tritanium", included: true }], state: "awaiting_delivery", version: 1, created_at: "2026-10-16T00:00:00Z" };
        pending.push(data);
      } else if (path.includes("/eve/contracts/")) {
        contractReads++;
        const entity = (id: string) => ({ id, name: id, category: "character" });
        data = { items: [{ id: "9001", title: "LNC-TEST-DELIVERY", type: "item_exchange", status: "finished", availability: "personal", for_corporation: false, issuer: entity("111"), assignee: entity("334"), acceptor: entity("334"), start: entity("6001"), end: entity("6001"), price: "0", reward: "0", collateral: "0", buyout: "0", volume: "1", days_to_complete: "0", date_issued: "2026-10-16T00:00:00Z", date_expired: "2026-10-20T00:00:00Z", date_accepted: "2026-10-16T01:00:00Z", date_completed: "2026-10-16T01:00:00Z", checked_at: "2026-10-16T01:01:00Z" }], next_cursor: "" };
      } else if (path.endsWith("/cases/1/collateral")) {
        expect(r.request().postDataJSON().request_id).toBe("1");
        expect(r.request().postDataJSON().contract_id).toBe(9001);
        expect(r.request().postDataJSON().haircut_bps).toBe(8000);
        pending[0].state = "delivered";
        data = { id: "1", case_id: "1", contract_id: "9001", valuation_minor: 375000, haircut_bps: 8000, covered_minor: 200000, state: "proposed", version: 1, items: pending[0].items };
        collateral.push(data);
      } else if (path.includes("/loan/")) data = { items: [] };
      return r.fulfill({ json: { data } });
    });
    await page.goto("/loans/operations?view=business");
    await page.getByRole("button", { name: tr("查看详情"), exact: true }).click();
    let dialog = page.getByRole("dialog");
    await expect(dialog.getByText(tr("还款安排"), { exact: true })).toBeVisible();
    await expect(dialog).toContainText("1,234,567,890,123.45 ISK");
    expect(contractReads).toBe(0);
    await dialog.getByRole("button", { name: tr("物品抵押"), exact: true }).click();
    dialog = page.getByRole("dialog");
    await expect(dialog.getByRole("combobox", { name: tr("贷款专员") })).toContainText("Officer Main");
    await dialog.getByRole("combobox", { name: tr("贷款专员") }).click();
    await page.getByRole("option", { name: "Officer Alt", exact: true }).click();
    await dialog.getByRole("checkbox", { name: /Tritanium/ }).check();
    await expect(dialog.getByRole("checkbox", { name: /Pyerite/ })).toBeDisabled();
    await expect(dialog.getByLabel(tr("抵押额度（ISK）"))).toHaveValue("2000");
    await expect(dialog.getByLabel(tr("抵押额度（ISK）"))).toBeEditable();
    await expect(dialog.getByRole("button", { name: tr("保存并查看交付信息"), exact: true })).toBeDisabled();
    await expect(dialog).toContainText(tr("还差 {0} 可抵额度，请增加物品或数量。").replace("{0}", "1,000 ISK"));
    await dialog.getByLabel(tr("抵押额度（ISK）")).fill("1000");
    await expect(dialog.getByRole("button", { name: tr("保存并查看交付信息"), exact: true })).toBeEnabled();
    await dialog.getByLabel(tr("抵押额度（ISK）")).fill("2000");
    await dialog.getByRole("spinbutton").fill("2");
    await expect(dialog.getByRole("button", { name: tr("保存并查看交付信息"), exact: true })).toBeEnabled();
    await expect(dialog).toContainText("2,500 ISK");
    await dialog.getByRole("spinbutton").fill("3");
    await dialog.getByRole("button", { name: tr("保存并查看交付信息"), exact: true }).click();
    await expect(dialog.getByRole("alert")).toHaveText("Assets changed; retry");
    await page.screenshot({ path: info.outputPath("asset-selection.png"), fullPage: true });
    await dialog.getByRole("button", { name: tr("保存并查看交付信息"), exact: true }).click();
    await expect(dialog.getByText(tr("待交付"), { exact: true })).toBeVisible();
    await expect(dialog.locator('input[value="Officer Alt"]')).toBeVisible();
    expect(await dialog.evaluate(e => e.scrollWidth <= e.clientWidth)).toBe(true);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.screenshot({ path: info.outputPath("pending-delivery.png"), fullPage: true });
    await dialog.getByRole("button", { name: tr("关联已完成合同"), exact: true }).click();
    await dialog.getByRole("combobox").selectOption("9001");
    await dialog.getByRole("button", { name: tr("核验并提交抵押"), exact: true }).click();
    await expect(dialog).toContainText("#9001");
    await expect(dialog.getByRole("button", { name: tr("批准抵押"), exact: true })).toHaveCount(0);
    await expect(dialog.getByText(tr("待交付"), { exact: true })).toHaveCount(0);
    await page.screenshot({ path: info.outputPath("collateral-details.png"), fullPage: true });
  });
}
