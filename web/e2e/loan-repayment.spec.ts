import { test, expect } from "@playwright/test";
import { english } from "../src/lib/locales/en";

for (const language of ["zh-CN", "en"]) {
  test(`loan repayment choices and dates (${language})`, async ({ page }, testInfo) => {
    const text = (s: string) => language === "en" ? english[s] ?? s : s;
    const submitted: Record<string, unknown>[] = [];
    await page.addInitScript(l => localStorage.setItem("glorynavy.locale", l), language);
    await page.route("https://images.evetech.net/**", r => r.abort());
    await page.route("**/api/v1/**", r => {
      const path = new URL(r.request().url()).pathname;
      let data: unknown = {};
      if (path.endsWith("/modules")) data = ["system", "identity", "eve", "access", "loan"].map(id => ({ id, version: "0.1.0", api_version: 1 }));
      else if (path.endsWith("/identity/session")) data = { authenticated: true, session: { user_id: "borrower", character: { id: "111", name: "Loan Pilot" }, csrf_token: "csrf", expires_at: "2099-01-01T00:00:00Z" } };
      else if (path.endsWith("/access/me")) data = { administrator: true, can_manage: true, can_manage_sync: false, site_roles: [], characters: [] };
      else if (path.endsWith("/identity/characters")) data = { characters: [{ id: "111", name: "Loan Pilot", status: "active", is_main: true }] };
      else if (path.endsWith("/community/profile")) data = { version: "1", complete: true, qq: { value: "10000", version: "1", confirmation: "confirmed" }, kook: { value: "Pilot", version: "1", confirmation: "confirmed" } };
      else if (path.endsWith("/loan/context")) data = { administrator: true, can_manage_pool: true, pools: [{ id: "1", name: "Shared pool", is_shared: true, lender_kind: "corporation", state: "open", config: {}, version: 1 }], credit: { score: 80, total_limit_minor: 100000000, state: "active" }, pool_summary: { pending_minor: 0, funded_minor: 0, cash_minor: 100000000, reserved_minor: 0, available_minor: 100000000 } };
      else if (path.endsWith("/loan/applications")) {
        expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
        submitted.push(r.request().postDataJSON());
      } else if (path.endsWith("/contracts/owners")) data = { owners: [] };
      else if (path.includes("/loan/")) data = { items: [] };
      return r.fulfill({ json: { data } });
    });
    await page.goto("/loans/operations?view=business");
    const open = async () => {
      await page.getByRole("button", { name: text("开始申请"), exact: true }).click();
      await page.getByLabel(text("本金（ISK）"), { exact: true }).fill("1000000");
    };
    const submit = async (expected: Record<string, unknown>) => {
      const count = submitted.length;
      await page.getByRole("button", { name: text("提交申请"), exact: true }).click();
      await expect.poll(() => submitted.length).toBe(count + 1);
      expect(submitted.at(-1)).toMatchObject(expected);
      await expect(page.getByRole("dialog")).toHaveCount(0);
    };
    await open();
    await expect(page.getByLabel(text("还款方式"), { exact: true })).toHaveValue("term");
    await submit({ installment_count: 1, interval_days: 30 });
    await open();
    await page.getByLabel(text("借款期限"), { exact: true }).selectOption("365");
    await submit({ installment_count: 1, interval_days: 365 });
    await open();
    await page.getByLabel(text("还款方式"), { exact: true }).selectOption("installments");
    await page.getByLabel(text("期数"), { exact: true }).fill("3");
    await page.getByLabel(text("间隔天数"), { exact: true }).selectOption("7");
    await submit({ installment_count: 3, interval_days: 7 });
    await open();
    await page.getByLabel(text("还款方式"), { exact: true }).selectOption("date");
    const dateInput = page.getByLabel(text("还款日期"), { exact: true });
    const selectedDate = (await dateInput.getAttribute("max"))!;
    await dateInput.fill(selectedDate);
    const dialog = page.getByRole("dialog");
    const bounds = await dialog.boundingBox();
    expect(bounds).not.toBeNull();
    expect(bounds!.x).toBeGreaterThanOrEqual(0);
    expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(page.viewportSize()!.width + 1);
    await expect(page.getByRole("button", { name: text("提交申请"), exact: true })).toBeInViewport();
    await page.screenshot({ path: testInfo.outputPath("repayment-date.png"), fullPage: true });
    await submit({ installment_count: 1, interval_days: 0, repayment_date: selectedDate });
    await open();
    await page.getByLabel(text("还款方式"), { exact: true }).selectOption("term");
    await page.getByLabel(text("借款期限"), { exact: true }).selectOption("custom");
    await page.getByLabel(text("借款天数（1–365）"), { exact: true }).fill("45");
    await submit({ installment_count: 1, interval_days: 45 });
    expect(submitted.at(-1)).not.toHaveProperty("repayment_date");
  });
}
