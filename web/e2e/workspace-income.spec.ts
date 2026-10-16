import { test, expect } from "@playwright/test";
import { english } from "../src/lib/locales/en";

for (const language of ["zh-CN", "en"]) {
  test(`workspace uses earnings without transfers and never falls back to gross cash flow (${language})`, async ({
    page,
  }, info) => {
    const tr = (s: string) => (language === "en" ? (english[s] ?? s) : s);
    await page.clock.setFixedTime(new Date("2026-10-16T00:00:00+08:00"));
    await page.addInitScript(
      (l) => localStorage.setItem("glorynavy.locale", l),
      language,
    );
    await page.route("https://images.evetech.net/**", (r) => r.abort());
    let mode = "complete";
    await page.route("**/api/v1/**", (r) => {
      const path = new URL(r.request().url()).pathname;
      let data: unknown = {};
      if (path.endsWith("/modules"))
        data = ["system", "identity", "eve", "access", "wallet"].map((id) => ({
          id,
          version: "0.1.0",
          api_version: 1,
        }));
      else if (path.endsWith("/identity/session"))
        data = {
          authenticated: true,
          session: {
            user_id: "owner",
            character: { id: "124", name: "Main Pilot" },
            main_character: { id: "124", name: "Main Pilot" },
            csrf_token: "csrf",
            expires_at: "2099-01-01T00:00:00Z",
          },
        };
      else if (path.endsWith("/access/me"))
        data = {
          administrator: false,
          can_manage: false,
          can_manage_sync: false,
          characters: [],
          site_roles: [],
        };
      else if (path.endsWith("/identity/characters"))
        data = {
          characters: [
            { id: "124", name: "Main Pilot", is_main: true, status: "active" },
            {
              id: "125",
              name: "Second Character With A Very Long Name",
              is_main: false,
              status: "active",
            },
          ],
        };
      else if (path.endsWith("/wallet/summary")) {
        const rows = [
          {
            owner_id: "124",
            balance: "1000.1",
            observed_at: "2026-10-15T00:00:00Z",
            income: "90000004500",
            expense: "80000000000",
            ...(mode !== "legacy"
              ? {
                  earned_income: mode === "zero" ? "0" : "4500",
                  spent_expense: "0",
                }
              : {}),
          },
          {
            owner_id: "125",
            balance: "0.2",
            observed_at: "2026-10-15T00:00:00Z",
            income: "80000000000",
            expense: "90000000150",
            ...(mode !== "legacy"
              ? {
                  earned_income: "0",
                  spent_expense: mode === "zero" ? "0" : "150",
                }
              : {}),
          },
        ];
        data = { items: mode === "partial" ? rows.slice(0, 1) : rows };
      }
      return r.fulfill({ json: { data } });
    });
    await page.goto("/workspace");
    const speeds = page.locator(".desk-wallet-speed strong");
    await expect(speeds).toHaveText(["300", "10"]);
    await expect(page.locator(".desk-isk strong")).toHaveText("1,000.30");
    await expect(page.locator(".desk-wallet-speed")).toContainText(
      tr("本月日均，不含转账"),
    );
    await expect(page.locator(".desk-review")).toHaveCount(0);
    await expect
      .poll(() =>
        page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
      )
      .toBe(true);
    await page.screenshot({
      path: info.outputPath("filtered-income.png"),
      fullPage: true,
    });
    mode = "zero";
    await page.reload();
    await expect(speeds).toHaveText(["0", "0"]);
    for (mode of ["legacy", "partial"]) {
      await page.reload();
      await expect(speeds).toHaveText(["—", "—"]);
    }
  });
}
