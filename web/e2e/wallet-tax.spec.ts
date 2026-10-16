import { test, expect } from "@playwright/test";
import { assertLayout } from "./state-layout";

for (const locale of ["zh-CN", "en"]) {
  test(`wallet withheld tax ${locale}`, async ({ page }, info) => {
    const en = locale === "en";
    await page.addInitScript((locale) => localStorage.setItem("glorynavy.locale", locale), locale);
    await page.route("https://images.evetech.net/**", (r) => r.abort());
    await page.route("**/api/v1/**", (r) => {
      const url = new URL(r.request().url());
      let data: unknown = {};
      if (url.pathname === "/api/v1/modules") data = ["system", "identity", "eve", "access", "wallet"].map((id) => ({ id, api_version: 1, version: "0.1.0" }));
      if (url.pathname === "/api/v1/identity/session") data = { authenticated: true, session: { user_id: "00000000-0000-4000-8000-000000000001", character: { id: "123", name: "Pilot" }, csrf_token: "test", expires_at: "2099-01-01T00:00:00Z" } };
      if (url.pathname === "/api/v1/access/me") data = { administrator: true, can_manage: true, can_manage_sync: true, site_roles: [], characters: [] };
      if (url.pathname === "/api/v1/wallet/context") data = { owners: [
        { kind: "character", id: "123", name: "Pilot", divisions: [0], journal: true, transactions: true },
        { kind: "corporation", id: "10", name: "Corporation", divisions: [1], journal: true, transactions: true },
      ] };
      if (url.pathname === "/api/v1/wallet/records") {
        const base = { division: Number(url.searchParams.get("division")), observed_at: "2026-10-17T00:00:00Z", date: "2026-10-16T23:00:00Z" };
        const taxed = { ...base, id: "53", amount: "3025360.79", tax: "432194.39875", tax_receiver_id: "10", ref_type: "bounty_prizes", first_party_id: "1000125", second_party_id: "123", balance: "10000000.01" };
        const debit = { ...base, id: "52", amount: "-123.45", ref_type: "transaction_tax" };
        const ordinary = { ...base, id: "51", amount: "200", tax: "0", ref_type: "player_donation" };
        const part = url.searchParams.get("part");
        const direction = url.searchParams.get("direction");
        data = { items: part === "balance" ? [{ ...base, id: "0", balance: "10000000.01" }] : part === "journal" ? (direction === "tax" ? [taxed] : direction === "out" ? [taxed, debit] : [taxed, debit, ordinary]) : [], names: { "123": "Pilot", "10": "TaxRecipientWithAnExtremelyLongCorporationNameForWrapping" }, next_cursor: "" };
      }
      return r.fulfill({ json: { data } });
    });
    await page.goto("/wallet");
    const table = page.getByRole("region", { name: en ? "Wallet records" : "钱包记录表格" });
    await expect(page.getByText(en ? "Tax withheld −432,194.39875" : "代扣税 −432,194.39875", { exact: true })).toBeVisible();
    await expect(page.getByText(en ? "Received 3,025,360.79" : "到账 3,025,360.79", { exact: true })).toBeVisible();
    const select = page.getByRole("combobox", { name: en ? "Direction" : "收支方向" });
    await select.click();
    await page.getByRole("option", { name: en ? "Expense" : "支出", exact: true }).click();
    const expenseRequest = page.waitForRequest((r) => new URL(r.url()).searchParams.get("direction") === "out");
    await page.getByRole("button", { name: en ? "Search records" : "查询记录", exact: true }).click();
    await expenseRequest;
    await expect(table.locator("tbody tr")).toHaveCount(2);
    await expect(table.locator(".wallet-amount-lines").first().locator(":scope > span").first()).toContainText(en ? "Tax withheld" : "代扣税");
    await select.click();
    await page.getByRole("option", { name: en ? "Tax" : "交税", exact: true }).click();
    const taxRequest = page.waitForRequest((r) => new URL(r.url()).searchParams.get("direction") === "tax");
    await page.getByRole("button", { name: en ? "Search records" : "查询记录", exact: true }).click();
    await taxRequest;
    await expect(table.locator("tbody tr")).toHaveCount(1);
    await expect(page.locator(".wallet-tax-help")).toContainText(en ? "already after tax" : "到账金额已扣税");
    await assertLayout(page);
    await table.evaluate((el) => { el.scrollLeft = el.scrollWidth; });
    await page.screenshot({ path: info.outputPath(`tax-${locale}.png`), fullPage: true });
    await page.getByRole("button", { name: en ? "View record 53" : "查看记录 53" }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByText("432,194.39875", { exact: true })).toBeVisible();
    await expect(dialog.getByText("TaxRecipientWithAnExtremelyLongCorporationNameForWrapping", { exact: true })).toBeVisible();
    await assertLayout(page);
    await page.screenshot({ path: info.outputPath(`detail-${locale}.png`), fullPage: true });
    await dialog.focus(); await page.keyboard.press("Escape");
    await page.getByRole("combobox", { name: en ? "Wallet owner" : "钱包归属" }).click();
    await page.getByRole("option", { name: "Corporation", exact: true }).click();
    await expect(page.locator(".wallet-withheld-tax")).toHaveCount(0);
    await select.click();
    await expect(page.getByRole("option", { name: en ? "Tax" : "交税", exact: true })).toHaveCount(0);
  });
}
