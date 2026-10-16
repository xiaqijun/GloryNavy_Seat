import { test, expect } from "@playwright/test";
import { english } from "../src/lib/locales/en";

for (const language of ["zh-CN", "en"]) {
  test(`withdraw pending applications and compact forms (${language})`, async ({ page }, testInfo) => {
    const text = (s: string) => language === "en" ? english[s] ?? s : s;
    const base = { pool_id: "1", pool_name: "Shared pool", lender_kind: "corporation", borrower_character_id: "111", principal_minor: 100000000, interest_minor: 100000, total_due_minor: 100100000, installment_count: 1, interval_days: 30, version: 1, created_at: "2026-10-16T00:00:00Z" };
    const cases = [
      { ...base, id: "1", public_id: "LN-OWN-PENDING", borrower_account_id: "borrower", state: "submitted" },
      { ...base, id: "2", public_id: "LN-OWN-APPROVED", borrower_account_id: "borrower", state: "approved" },
      { ...base, id: "3", public_id: "LN-OTHER-PENDING", borrower_account_id: "other", state: "submitted" },
    ];
    const requests: unknown[] = [];
    const contributions: unknown[] = [];
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
      else if (path.endsWith("/loan/context")) data = { administrator: true, can_manage_pool: true, pools: [{ id: "1", name: "Shared pool", is_shared: true, lender_kind: "corporation", state: "open", config: {}, version: 1 }], credit: { score: 80, total_limit_minor: 100000000, state: "active" }, pool_summary: { pending_minor: 0, funded_minor: 0, cash_minor: 100000000, reserved_minor: 0, available_minor: 100000000 } };
      else if (path.endsWith("/loan/cases")) data = { items: cases };
      else if (path.endsWith("/cases/1")) data = { case: cases[0], guarantees: [], collateral: [], installments: [] };
      else if (path.endsWith("/cases/1/cancel")) {
        expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
        requests.push(r.request().postDataJSON());
        if (requests.length === 1) {
          cases[0].version = 2;
          return r.fulfill({ status: 409, json: { error: { code: "loan_conflict", message: "Refresh and retry" } } });
        }
        cases[0].state = "cancelled";
        cases[0].version++;
        data = cases[0];
      } else if (path.endsWith("/loan/contributions") && r.request().method() === "POST") {
        contributions.push(r.request().postDataJSON());
      } else if (path.endsWith("/contracts/owners")) data = { owners: [] };
      else if (path.includes("/loan/")) data = { items: [] };
      return r.fulfill({ json: { data } });
    });
    await page.goto("/loans/operations?view=business");
    const pending = page.getByRole("row").filter({ hasText: "LN-OWN-PENDING" });
    const withdraw = pending.getByRole("button", { name: text("撤销申请"), exact: true });
    await expect(page.getByRole("button", { name: text("撤销申请"), exact: true })).toHaveCount(1);
    await withdraw.scrollIntoViewIfNeeded();
    await expect(withdraw).toBeInViewport();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    await page.screenshot({ path: testInfo.outputPath("loan-records.png"), fullPage: true });
    await withdraw.click();
    const dialog = page.getByRole("alertdialog");
    await expect(dialog.getByRole("button", { name: text("取消"), exact: true })).toBeFocused();
    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);
    expect(requests).toHaveLength(0);
    await expect(withdraw).toBeFocused();
    await pending.getByRole("button", { name: text("查看详情"), exact: true }).click();
    await page.getByRole("dialog").getByRole("button", { name: text("撤销申请"), exact: true }).click();
    await dialog.getByRole("button", { name: text("确认撤销申请"), exact: true }).click();
    await expect(dialog.getByRole("alert")).toHaveText("Refresh and retry");
    await dialog.getByRole("button", { name: text("确认撤销申请"), exact: true }).click();
    await expect(dialog).toHaveCount(0);
    await expect(pending).toContainText(text("已取消"));
    await expect(withdraw).toHaveCount(0);
    expect(requests).toEqual([{ version: 1 }, { version: 2 }]);
    await page.getByRole("button", { name: text("出借资金"), exact: true }).click();
    await page.getByLabel(text("出借额度（ISK）"), { exact: true }).fill("1000");
    await page.getByLabel(text("出借类型"), { exact: true }).selectOption("corporation");
    await page.getByLabel(text("出借军团 ID"), { exact: true }).fill("98000001");
    await expect(page.getByRole("button", { name: text("提交出借"), exact: true })).toBeInViewport();
    await page.screenshot({ path: testInfo.outputPath("contribution-form.png"), fullPage: true });
    await page.getByRole("button", { name: text("提交出借"), exact: true }).click();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    expect(contributions).toEqual([{ lender_kind: "corporation", source_character_id: 111, corporation_id: 98000001, amount_minor: 100000 }]);
  });
}
