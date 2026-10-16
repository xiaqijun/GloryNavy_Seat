import { test, expect } from "@playwright/test";
import { english } from "../src/lib/locales/en";

for (const language of ["zh-CN", "en"]) {
  test(`one personal limit and an above-limit application (${language})`, async ({ page }, info) => {
    const tr = (s: string) => language === "en" ? english[s] ?? s : s;
    const credit = { score: 80, total_limit_minor: 1_000_000_000_000, unsecured_limit_minor: 300_000_000_000, state: "active" };
    let submitted: Record<string, unknown> | undefined;
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
      else if (path.endsWith("/loan/context")) data = { administrator: false, can_manage_pool: false, pools: [{ id: "1", name: "Shared pool", is_shared: true, lender_kind: "corporation", state: "open", config: {}, version: 1 }], credit };
      else if (path.endsWith("/loan/evaluations")) data = { items: [{ ...credit, id: "1", policy_version: "capacity-v6-policy-1", evaluated_at: "2026-10-16T00:00:00Z" }] };
      else if (path.endsWith("/loan/applications")) {
        expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
        submitted = r.request().postDataJSON();
      } else if (path.includes("/loan/")) data = { items: [] };
      return r.fulfill({ json: { data } });
    });
    await page.goto("/loans/operations?view=evaluations");
    const table = page.getByRole("table");
    await expect(table.getByRole("columnheader")).toHaveCount(5);
    await expect(table.getByRole("columnheader", { name: tr("个人额度"), exact: true })).toBeVisible();
    await expect(table.getByRole("columnheader", { name: tr("总额度"), exact: true })).toHaveCount(0);
    await expect(table.getByRole("columnheader", { name: tr("无担保额度"), exact: true })).toHaveCount(0);
    await expect(table).toContainText("10B ISK");
    await expect(table).not.toContainText("3B ISK");
    await expect(page.getByText(tr("借款超出剩余个人额度的部分，需要担保或抵押。评估记录保留当时的评分、额度和规则版本。"))).toBeVisible();
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.screenshot({ path: info.outputPath("single-credit-evaluation.png"), fullPage: true });
    await page.getByRole("tab", { name: tr("贷款办理"), exact: true }).click();
    await page.getByRole("button", { name: tr("申请借款"), exact: true }).click();
    await page.getByLabel(tr("本金（ISK）"), { exact: true }).fill("12000000000");
    await page.getByRole("button", { name: tr("提交申请"), exact: true }).click();
    await expect.poll(() => submitted).toMatchObject({ principal_minor: 1_200_000_000_000, installment_count: 1, interval_days: 30 });
    await expect(page.getByRole("dialog")).toHaveCount(0);
  });
}
