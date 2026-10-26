import { test, expect, type Page } from "@playwright/test";

const owner = "00000000-0000-4000-8000-000000000001";
async function setup(page: Page, language: string) {
  let actor = owner, staff = false, requests = 0;
  const ticket: Record<string, unknown> = {
    id: "1", account_id: owner, author_name: "Pilot", kind: "bug", title: "撤销流程测试",
    body: "保留原始反馈和截图证据", status: "submitted", revision: 1,
    created_at: "2026-10-26T00:00:00Z", updated_at: "2026-10-26T00:00:00Z",
  };
  const messages: unknown[] = [];
  let release: (() => void) | undefined;
  let pending: Promise<void> | undefined;
  await page.addInitScript((value) => localStorage.setItem("glorynavy.locale", value), language);
  await page.route("https://images.evetech.net/**", (route) => route.abort());
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url()), path = url.pathname;
    let data: unknown = {};
    if (path === "/api/v1/modules") data = ["system", "identity", "eve", "access", "feedback"].map((id) => ({ id, api_version: 1, version: "0.1.0" }));
    if (path === "/api/v1/identity/session") data = { authenticated: true, session: { user_id: actor,
      character: { id: "123", name: "Pilot" }, csrf_token: "csrf", expires_at: "2099-01-01T00:00:00Z" } };
    if (path === "/api/v1/access/me") data = { administrator: staff, can_manage: staff, site_roles: [], characters: [] };
    if (path === "/api/v1/feedback/tickets") data = { tickets: !url.searchParams.get("status") || url.searchParams.get("status") === ticket.status ? [ticket] : [], next_before: "", administrator: staff };
    if (path === "/api/v1/feedback/tickets/1") data = { ticket, messages, administrator: staff };
    if (path.endsWith("/messages")) {
      const input = route.request().postDataJSON();
      expect(input.revision).toBe(ticket.revision);
      expect(route.request().headers()["x-csrf-token"]).toBe("csrf");
      expect(input).not.toHaveProperty("reward_minor");
      requests++;
      if (pending) await pending;
      ticket.status = input.status;
      ticket.revision = Number(ticket.revision) + 1;
      messages.push({ id: String(messages.length + 1), author_name: "Pilot", staff: false, body: input.body, status: input.status, created_at: ticket.updated_at });
      data = ticket;
    }
    await route.fulfill({ json: { data } });
  });
  return { ticket, get requests() { return requests; }, setActor(id: string, admin: boolean) { actor = id; staff = admin; },
    hold() { pending = new Promise<void>((resolve) => { release = resolve; }); }, release() { release?.(); pending = undefined; } };
}

for (const language of ["zh-CN", "en"]) {
  const en = language === "en";
  test(`feedback owner withdraws and resubmits without approval ${language}`, async ({ page }, info) => {
    const fixture = await setup(page, language);
    await page.goto("/feedback?ticket=1");
    const dialog = page.getByRole("dialog");
    const withdraw = dialog.getByRole("button", { name: en ? "Withdraw feedback" : "撤销反馈", exact: true });
    await withdraw.click();
    await expect(dialog.getByText(en ? "Withdrawal stops handling. The original record is retained." : "撤销后停止处理，原记录会保留。", { exact: true })).toBeVisible();
    await dialog.getByRole("button", { name: en ? "Cancel" : "取消", exact: true }).click();
    expect(fixture.requests).toBe(0);
    await withdraw.click();
    await page.screenshot({ path: info.outputPath(`confirm-withdraw-${language}.png`), fullPage: true });
    fixture.hold();
    const confirm = dialog.getByRole("button", { name: en ? "Confirm withdrawal" : "确认撤销反馈", exact: true });
    await confirm.click();
    await expect(dialog.locator('button[form="feedback-action-1"]')).toBeDisabled();
    await page.keyboard.press("Escape");
    await expect(dialog).toBeVisible();
    fixture.release();
    await expect(dialog.locator(".feedback-status")).toHaveText(en ? "Withdrawn" : "已撤销");
    await expect(dialog.getByRole("button", { name: en ? "Resubmit" : "重新提交", exact: true })).toBeVisible();
    await expect(dialog.getByRole("button", { name: en ? "Withdraw feedback" : "撤销反馈", exact: true })).toHaveCount(0);
    await expect(dialog.getByText("保留原始反馈和截图证据", { exact: true })).toBeVisible();
    expect(fixture.requests).toBe(1);
    await page.screenshot({ path: info.outputPath(`withdrawn-${language}.png`), fullPage: true });
    await dialog.getByRole("button", { name: en ? "Resubmit" : "重新提交", exact: true }).click();
    const save = dialog.getByRole("button", { name: en ? "Confirm action" : "确认操作", exact: true });
    await expect(save).toBeDisabled();
    await dialog.getByRole("textbox").fill("补充复现信息后重新提交");
    await save.click();
    await expect(withdraw).toBeVisible();
    expect(fixture.requests).toBe(2);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  });

  test(`feedback withdrawal is limited to unadopted current-owner submissions ${language}`, async ({ page }) => {
    const fixture = await setup(page, language);
    const button = page.getByRole("button", { name: en ? "Withdraw feedback" : "撤销反馈", exact: true });
    for (const state of ["accepted", "in_progress", "awaiting_confirmation", "resolved", "closed"]) {
      fixture.ticket.status = state;
      await page.goto("/feedback?ticket=1");
      await expect(page.getByRole("dialog")).toBeVisible();
      await expect(button).toHaveCount(0);
    }
    fixture.ticket.status = "submitted";
    fixture.ticket.accepted_at = "2026-10-26T00:00:00Z";
    fixture.ticket.reward_minor = 20000;
    await page.goto("/feedback?ticket=1");
    await expect(page.getByRole("dialog")).toBeVisible();
    await expect(button).toHaveCount(0);
    delete fixture.ticket.accepted_at;
    delete fixture.ticket.reward_minor;
    fixture.setActor("00000000-0000-4000-8000-000000000002", true);
    await page.goto("/feedback?ticket=1");
    await expect(page.getByRole("dialog")).toBeVisible();
    await expect(button).toHaveCount(0);
    fixture.setActor(owner, true);
    await page.goto("/feedback?ticket=1");
    await expect(button).toBeVisible();
    expect(fixture.requests).toBe(0);
  });
}
