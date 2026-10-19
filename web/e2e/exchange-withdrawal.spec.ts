import { test, expect, type Page } from "@playwright/test";

async function fixture(
  page: Page,
  mode: "pending" | "requested" | "started" | "admin" = "pending",
) {
  let state = mode === "requested" ? "cancel_requested" : "pending";
  let started = mode === "started";
  let balance = -41543;
  const writes: Record<string, unknown>[] = [];
  await page.route("https://images.evetech.net/**", (r) => r.abort());
  await page.route("**/api/v1/**", async (r) => {
    const path = new URL(r.request().url()).pathname;
    let data: unknown;
    if (path.endsWith("/modules"))
      data = ["system", "identity", "eve", "access", "exchange"].map((id) => ({
        id,
        version: "0.1.0",
        api_version: 1,
      }));
    else if (path.endsWith("/identity/session"))
      data = {
        authenticated: true,
        session: {
          user_id: "user",
          character: { id: "123", name: "测试角色" },
          csrf_token: "csrf",
          expires_at: "2099-01-01T00:00:00Z",
        },
      };
    else if (path.endsWith("/access/me"))
      data = {
        administrator: mode === "admin",
        can_manage: false,
        can_manage_sync: false,
        site_roles: [],
        characters: [],
      };
    else if (path.endsWith("/exchange/context"))
      data = { characters: [{ id: "123", name: "测试角色" }], sources: [] };
    else if (path.endsWith("/exchange/wallet"))
      data = { items: [], next_cursor: "" };
    else if (path.endsWith("/handoff")) {
      if (r.request().method() === "POST") {
        expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
        writes.push(r.request().postDataJSON());
        started = true;
      }
      data = {
        started,
        can_start: true,
        reference: "EX-20261019-000021",
        recipient_id: "123",
        recipient_name: "测试角色",
        amount: "123",
        state,
      };
    } else if (path.endsWith("/orders/1")) {
      const body = r.request().postDataJSON();
      writes.push(body);
      expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
      expect(body.request_key).toMatch(/^[0-9a-f-]{36}$/);
      if (body.state === "withdrawn") {
        expect(started).toBe(false);
        balance += 50000;
        state = "cancelled";
      } else {
        expect(body.state).toBe("cancel_requested");
        state = "cancel_requested";
      }
      data = { saved: true };
    } else if (path.endsWith("/rewards/orders"))
      data = {
        items: [
          {
            id: "1",
            version: "1",
            name: "三钛合金",
            type_id: "34",
            quantity: 50,
            units: 5,
            recipient_id: "123",
            recipient_name: "测试角色",
            coins_minor: 50000,
            isk_per_coin: 100,
            isk_value: 50000,
            state,
            can_request_cancel: mode !== "admin" && state === "pending",
            can_withdraw: mode !== "admin" && state !== "cancelled" && !started,
            reference: "EX-20261019-000021",
            delivery: {
              started,
              status: "waiting_contract",
              checked_at: null,
              contracts: [],
            },
            note: "",
            created_at: "2026-10-19T09:00:00Z",
          },
        ],
        next_cursor: "",
      };
    else if (path.endsWith("/exchange/rewards"))
      data = {
        admin: mode === "admin",
        isk_per_coin: 100,
        version: "1",
        earned_minor: 8457,
        reserved_minor: state === "cancelled" ? 0 : 50000,
        spent_minor: 0,
        available_minor: balance,
        rewards: [],
        next_cursor: "",
      };
    else return r.fulfill({ status: 404, json: { error: { message: path } } });
    await r.fulfill({ json: { data } });
  });
  await page.goto("/exchange");
  return writes;
}

for (const mode of ["pending", "requested"] as const) {
  test(`${mode}: withdraw immediately and refund all packs`, async ({
    page,
  }, info) => {
    const writes = await fixture(page, mode);
    await expect(
      page.locator(".exchange-metric").filter({ hasText: "可用果壳币" }),
    ).toContainText("-415.43");
    await page.getByRole("button", { name: "撤销兑换", exact: true }).click();
    await expect(page.getByRole("textbox", { name: "取消原因" })).toHaveCount(
      0,
    );
    await expect(page.locator(".exchange-refund-summary")).toContainText("500");
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    if (mode === "requested")
      await page.screenshot({
        path: `.local/exchange-withdraw/${info.project.name}-withdraw.png`,
      });
    await page.getByRole("button", { name: "确认撤销", exact: true }).click();
    await expect(page.getByText("#1 · 已取消")).toBeVisible();
    await expect(
      page.locator(".exchange-metric").filter({ hasText: "可用果壳币" }),
    ).toContainText("84.57");
    await expect(
      page.getByRole("button", { name: "撤销兑换", exact: true }),
    ).toHaveCount(0);
    expect(writes).toHaveLength(1);
    expect(writes[0].state).toBe("withdrawn");
  });
}

test("started order retains cancellation review", async ({ page }) => {
  await fixture(page, "started");
  await expect(
    page.getByRole("button", { name: "撤销兑换", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "申请取消", exact: true }).click();
  await page.getByLabel("取消原因").fill("暂不领取");
  await page.getByRole("button", { name: "提交申请", exact: true }).click();
  await expect(page.getByText("#1 · 取消待审核")).toBeVisible();
  await expect(
    page.locator(".exchange-metric").filter({ hasText: "可用果壳币" }),
  ).toContainText("-415.43");
});

test("contract handoff requires explicit start", async ({ page }) => {
  const writes = await fixture(page, "admin");
  await page.getByRole("button", { name: "合同信息", exact: true }).click();
  await expect(page.getByRole("button", { name: "复制接收角色" })).toHaveCount(
    0,
  );
  expect(writes).toHaveLength(0);
  await page.getByRole("button", { name: "开始发放", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "复制接收角色" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "复制合同结算 ID" }),
  ).toBeVisible();
  expect(writes).toEqual([{ version: "1" }]);
});
