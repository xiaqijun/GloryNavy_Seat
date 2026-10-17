import { test, expect, type Page } from "@playwright/test";
import { english } from "../src/lib/locales/en";
import type { Role } from "../src/modules/access/manage-api";

const operator = "01994763-4111-7000-8000-111111111111";
const member = "01994763-4111-7000-8000-222222222222";
const presets = [
  ["补损专员", "corporation.welfare.compensation", "补损审核与交付"],
  ["福利发放专员", "corporation.welfare", "福利审核与交付"],
  ["贷款专员", "corporation.loan", "贷款管理"],
] as const;

async function fixture(page: Page, allowed = true) {
  const state = { roles: [] as Role[], assigned: [] as string[], writes: 0 };
  await page.route("https://images.evetech.net/**", (route) => route.abort());
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request(), path = new URL(request.url()).pathname;
    let data: unknown = {};
    if (path.endsWith("/modules")) data = ["system", "identity", "eve", "access", "loan"].map(id => ({ id, version: "0.1.0", api_version: 1 }));
    else if (path.endsWith("/identity/session")) data = { authenticated: true, session: { user_id: operator, character: { id: "111", name: "权限管理员" }, csrf_token: "csrf", expires_at: "2099-01-01T00:00:00Z" } };
    else if (path.endsWith("/access/me")) data = { administrator: allowed, can_manage: allowed, site_roles: [], characters: [] };
    else if (path.endsWith("/access/catalog")) data = presets.map(([,id,label]) => ({ id, label, scope: "corporation" }));
    else if (path.endsWith("/access/roles")) data = state.roles;
    else if (/\/access\/roles\//.test(path)) {
      expect(request.headers()["x-csrf-token"]).toBe("csrf");
      state.writes++;
      const id = path.split("/").at(-1)!;
      const body = request.postDataJSON();
      const old = state.roles.find(role => role.id === id);
      expect(body.version).toBe(old?.version ?? "0");
      state.roles = [...state.roles.filter(role => role.id !== id), { ...body, id, version: String(Number(body.version) + 1) }];
      data = { saved: true };
    } else if (path.endsWith("/access/members")) data = { items: [{ user_id: member, character_id: "222", name: "配置测试成员", character_count: 3, administrator: false, roles: state.roles.filter(role => state.assigned.includes(role.id)) }], next: "" };
    else if (path.startsWith(`/api/v1/access/users/${member}/roles/`)) {
      expect(request.headers()["x-csrf-token"]).toBe("csrf");
      const id = path.split("/").at(-1)!;
      state.assigned = request.method() === "PUT" ? [...state.assigned, id] : state.assigned.filter(role => role !== id);
      data = { saved: true };
    } else if (path.endsWith("/access/audit")) data = { items: [], next: "" };
    else if (path.endsWith("/community/profile")) data = { version: "1", complete: true, qq: { value: "10000", version: "1", confirmation: "confirmed" }, kook: { value: "Pilot", version: "1", confirmation: "confirmed" } };
    else if (path.endsWith("/identity/characters")) data = { characters: [{ id: "111", name: "Contract Pilot", status: "active", is_main: true }] };
    else if (path.endsWith("/loan/context")) data = { administrator: allowed, can_manage_pool: true, pools: [{ id: "1", name: "Shared Pool", is_shared: true, lender_kind: "corporation", state: "open", config: {}, version: 1, custodian_character_id: "111", custodian_character_name: "Contract Pilot" }], credit: { state: "active", total_limit_minor: 0 } };
    else if (path.endsWith("/loan/settings/policy")) data = { can_manage: true, policy: { income_window_days: 30, annual_rate_bps: 100, term_months: 6, debt_ratio_bps: 1000, score_factor_bps: 10000, income_weight_bps: 7000, asset_credit_ltv_bps: 1000, guarantee_cap_bps: 10000, collateral_cap_bps: 8000, stale_after_hours: 72, min_income_samples: 1, insufficient_income_policy: "zero", version: 1, updated_at: "2026-10-17T00:00:00Z" } };
    else if (path.startsWith("/api/v1/loan/")) data = { items: [] };
    return route.fulfill({ json: { data } });
  });
  return state;
}

for (const language of ["zh-CN", "en"]) {
  test(`officer grants, reuse, assignment and revocation (${language})`, async ({ page }, info) => {
    const tr = (value: string) => language === "en" ? english[value] ?? value : value;
    await page.addInitScript(lang => localStorage.setItem("glorynavy.locale", lang), language);
    const state = await fixture(page);
    await page.goto("/access");
    const shortcuts = page.getByRole("region", { name: tr("专员配置") });
    await expect(shortcuts.getByRole("button")).toHaveCount(3);
    await expect(page.getByText(/PAP.*专员|PAP.*officer/i)).toHaveCount(0);
    for (const [name, permission] of presets) {
      await shortcuts.getByRole("button", { name: tr(name), exact: true }).focus();
      await page.keyboard.press("Enter");
      await expect(page.getByLabel(tr("角色名称"), { exact: true })).toHaveValue(tr(name));
      await expect(page.getByRole("checkbox", { checked: true })).toHaveCount(1);
      await page.getByRole("button", { name: tr("保存角色"), exact: true }).click();
      await expect(page.getByRole("status")).toHaveText(tr("角色已保存"));
      expect(state.roles.find(role => role.name === tr(name))?.grants).toEqual([{ permission, corporations: [], alliances: [] }]);
    }
    await shortcuts.getByRole("button", { name: tr("贷款专员"), exact: true }).click();
    await expect(page.getByRole("button", { name: tr("配置成员"), exact: true })).toBeVisible();
    expect(state.roles).toHaveLength(3);
    expect(state.writes).toBe(3);
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.screenshot({ path: info.outputPath(`officers-${language}.png`), fullPage: true });
    await page.getByRole("button", { name: tr("配置成员"), exact: true }).click();
    await page.getByRole("button", { name: /配置测试成员/ }).click();
    const loan = state.roles.find(role => role.name === tr("贷款专员"))!;
    await page.getByLabel(tr("授予角色"), { exact: true }).selectOption(loan.id);
    await page.getByRole("button", { name: tr("授予"), exact: true }).click();
    await expect(page.getByRole("status")).toHaveText(tr("角色已授予"));
    expect(state.assigned).toEqual([loan.id]);
    await page.getByRole("button", { name: tr("撤销{0}").replace("{0}", loan.name), exact: true }).click();
    await page.getByRole("alertdialog").getByRole("button", { name: tr("确认"), exact: true }).click();
    await expect(page.getByRole("status")).toHaveText(tr("角色已撤销"));
    expect(state.assigned).toEqual([]);
    // Existing translated names and corporation restrictions survive locale changes.
    loan.grants[0].corporations = ["12345"];
    const other = language === "en" ? "zh-CN" : "en";
    await page.goto(`/access?lang=${other}`);
    const loanLabel = other === "en" ? english["贷款专员"] : "贷款专员";
    await page.getByRole("region", { name: other === "en" ? english["专员配置"] : "专员配置" })
      .getByRole("button", { name: loanLabel, exact: true }).click();
    await expect(page.getByLabel(other === "en" ? english["角色名称"] : "角色名称", { exact: true })).toHaveValue(loan.name);
    await page.getByRole("button", { name: other === "en" ? english["保存角色"] : "保存角色", exact: true }).click();
    await expect(page.getByRole("status")).toHaveText(other === "en" ? english["角色已保存"] : "角色已保存");
    expect(state.roles).toHaveLength(3);
    expect(state.roles.find(role => role.id === loan.id)?.grants).toEqual(loan.grants);
  });
}

test("loan officer authorization is separate from contract character", async ({ page }, info) => {
  await fixture(page);
  await page.goto("/loans/operations?view=risk");
  await expect(page.getByRole("link", { name: "配置贷款专员", exact: true })).toHaveAttribute("href", "/access");
  await page.getByRole("button", { name: "设置合同处理角色", exact: true }).click();
  await expect(page.getByRole("dialog", { name: "设置合同处理角色" })).toBeVisible();
  await expect(page.getByRole("button", { name: "设置贷款专员", exact: true })).toHaveCount(0);
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath("loan-contract-character.png"), fullPage: true });
});

test("ordinary member cannot configure officers", async ({ page }) => {
  const state = await fixture(page, false);
  await page.goto("/access");
  await expect(page.getByText("没有权限管理权限", { exact: true })).toBeVisible();
  await expect(page.getByRole("region", { name: "专员配置" })).toHaveCount(0);
  expect(state.writes).toBe(0);
});
