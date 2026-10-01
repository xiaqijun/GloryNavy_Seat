import { stateLayouts } from "./state-layout";
import { navigate } from "./navigation";
import { test, expect, type Page } from "@playwright/test";
import { englishLayout } from "./language-layout";
import { mkdirSync } from "node:fs";
const user = "00000000-0000-4000-8000-000000000001",
  member = "00000000-0000-4000-8000-000000000002";
const config = {
  enabled: true,
  effective_at: "2020-01-01T00:00:00Z",
  ship_type_id: "17715",
  fitting_id: "1",
  skill_plan_id: "1",
  reference_minor: 50000000000,
  day_zone: "UTC",
  note: "已公告规则",
};
const detail = {
  character_id: "123",
  character_name: "测试成员",
  ship_type_id: "17715",
  killmail_id: "987",
  contract_id: "456",
  event_id: "1",
  occurred_at: "2026-09-01T12:00:00Z",
  description: "舰队行动中损失",
  evidence: "KM 与购舰合同已提交",
  alliance: "unknown",
  base_minor: 0,
  discipline: false,
  policy_version: "1",
  rule: config,
  receipt: "",
  reviewer: "",
  executor: "",
  review_note: "",
};
const syncedLoss = {
  reimbursement: {
    status: "available",
    state: "",
    kind: "",
    attendance_event_id: "1",
    available_kinds: ["srp", "alliance", "solo"],
  },
  id: "987",
  character_id: "123",
  corporation_id: "10",
  ship_type_id: "17715",
  ship_name: "毒蜥级",
  solar_system_id: "30000142",
  solar_system_name: "吉他",
  occurred_at: "2026-09-01T12:00:00Z",
  observed_at: "2026-09-16T01:00:00Z",
  damage_taken: 8000,
  attackers: [
    { character_id: "901", name: "最后一击角色", corporation_name: "测试军团", ship_type_id: "587", ship_name: "裂谷级", damage_done: 5000, final_blow: true },
    { character_id: "902", name: "协同角色", ship_type_id: "597", ship_name: "惩罚者级", damage_done: 3000, final_blow: false },
  ],
  items: [
    { type_id: "100", name: "高槽装备", slot: "27", quantity: 1, destroyed: 1, dropped: 0 },
    { type_id: "101", name: "中槽装备", slot: "19", quantity: 1, destroyed: 0, dropped: 1 },
    { type_id: "102", name: "低槽装备", slot: "11", quantity: 1, destroyed: 1, dropped: 0 },
    { type_id: "103", name: "改装件", slot: "92", quantity: 1, destroyed: 1, dropped: 0 },
    { type_id: "104", name: "无人机", slot: "87", quantity: 2, destroyed: 0, dropped: 2 },
    { type_id: "3293", name: "标准货柜", slot: "5", quantity: 1, destroyed: 0, dropped: 1 },
    {
      type_id: "34",
      name: "三钛合金",
      slot: "5/5",
      quantity: 5,
      destroyed: 2,
      dropped: 3,
    },
  ],
};

test("旗舰 ISK 申请只填购舰合同并保留失败重试", async ({ page }, info) => {
  await setup(page, false);
  await page.route("**/api/v1/welfare/context**", (r) =>
    r.fulfill({
      json: {
        data: {
          administrator: false,
          corporations: [{ id: "10", name: "荣耀海军", can_manage: false }],
          characters: [
            {
              id: "123",
              name: "测试成员",
              account_id: member,
              corporation_id: "10",
            },
          ],
          policies: ["supercarrier", "titan"].map((kind) => ({
            corporation_id: "10",
            kind,
            version: "1",
            config: { ...config, effective_at: "2099-01-01T00:00:00Z" },
          })),
        },
      },
    }),
  );
  await page.reload();
  await page.getByRole("button", { name: "旗舰补贴", exact: true }).click();
  await page.getByRole("button", { name: "申请", exact: true }).click();
  const modal = page.getByRole("dialog");
  await expect(modal.getByLabel("购舰合同 ID", { exact: true })).toBeVisible();
  await expect(modal.getByRole("combobox")).toHaveCount(0);
  await expect(modal.getByRole("textbox")).toHaveCount(1);
  await expect(
    modal.getByRole("button", { name: "提交申请", exact: true }),
  ).toBeDisabled();
  await modal.getByLabel("购舰合同 ID", { exact: true }).fill("901");
  const writes: any[] = [];
  await page.route("**/api/v1/welfare/commands", (r) => {
    writes.push(r.request().postDataJSON());
    return writes.length === 1
      ? r.fulfill({
          status: 409,
          json: {
            error: {
              code: "welfare_purchase_required",
              message: "请同步已完成的个人购舰合同",
            },
          },
        })
      : r.fulfill({ json: { data: {} } });
  });
  await modal.getByRole("button", { name: "提交申请", exact: true }).click();
  await expect(modal.getByRole("alert")).toContainText("请同步");
  await expect(modal.getByLabel("购舰合同 ID", { exact: true })).toHaveValue(
    "901",
  );
  expect(
    await page.evaluate(
      () =>
        document.documentElement.scrollWidth <=
        document.documentElement.clientWidth,
    ),
  ).toBe(true);
  mkdirSync("../docs/ui/reviews/welfare", { recursive: true });
  await page.screenshot({
    path: `../docs/ui/reviews/welfare/capital-apply-${info.project.name}.png`,
    fullPage: true,
  });
  await modal.getByRole("button", { name: "提交申请", exact: true }).click();
  expect(writes[0].detail).toEqual({ contract_id: "901" });
  expect(writes[1].request_key).toBe(writes[0].request_key);
  await expect(modal).toHaveCount(0);
});

test("旗舰资格精简后保存保留历史月份", async ({ page }) => {
  await setup(page);
  const profile = {
    account_id: member,
    version: "2",
    verified: true,
    history: { growth_gila: "used", supercarrier: "unused", titan: "unknown" },
    months: ["2026-08"],
  };
  await page.route("**/api/v1/welfare/profile?**", (r) =>
    r.fulfill({ json: { data: profile } }),
  );
  await page.getByRole("button", { name: "旗舰补贴", exact: true }).click();
  await page.getByRole("button", { name: "旗舰资格", exact: true }).click();
  const modal = page.getByRole("dialog", { name: "旗舰资格", exact: true });
  await expect(
    modal.getByLabel("已确认 Active 月（YYYY-MM，逗号分隔）"),
  ).toHaveCount(0);
  await expect(modal.getByLabel("核验依据")).toHaveCount(0);
  await expect(
    modal.getByRole("checkbox", { name: "成员身份已核实" }),
  ).toHaveCount(0);
  await expect(modal.getByRole("combobox", { name: /历史资格/ })).toHaveCount(
    2,
  );
  const posted = page.waitForRequest(
    (r) => r.url().endsWith("/welfare/commands") && r.method() === "POST",
  );
  await modal.getByRole("button", { name: "保存", exact: true }).click();
  const body = (await posted).postDataJSON();
  expect(body.profile).toEqual(profile);
  expect(body.note).toBe("更新旗舰领取资料");
  await expect(modal).toHaveCount(0);
});

test("旗舰比例可配置且切换类别各自读取", async ({ page }) => {
  await setup(page);
  await page.getByRole("button", { name: "旗舰补贴", exact: true }).click();
  await page.getByRole("button", { name: "旗舰补贴规则", exact: true }).click();
  const modal = page.getByRole("dialog");
  await expect(modal.getByLabel("补贴比例（%）")).toHaveValue("10");
  await modal.getByLabel("补贴比例（%）").fill("12.5");
  await modal.getByRole("combobox", { name: "规则项目" }).click();
  await page.getByRole("option", { name: "泰坦", exact: true }).click();
  await expect(modal.getByLabel("补贴比例（%）")).toHaveValue("5");
  await modal.getByLabel("补贴比例（%）").fill("7.25");
  const posted = page.waitForRequest(
    (r) => r.url().endsWith("/welfare/commands") && r.method() === "POST",
  );
  await modal.getByRole("button", { name: "保存", exact: true }).click();
  const body = (await posted).postDataJSON();
  expect(body.kind).toBe("titan");
  expect(body.config.subsidy_rate_bps).toBe(725);
  await expect(modal).toHaveCount(0);
});

test("旗舰规则无需填写生效时间", async ({ page }) => {
  await setup(page);
  await page.getByRole("button", { name: "旗舰补贴", exact: true }).click();
  await page.getByRole("button", { name: "旗舰补贴规则", exact: true }).click();
  const modal = page.getByRole("dialog");
  await expect(modal.getByLabel("生效时间（含时区）")).toHaveCount(0);
  await modal.getByRole("checkbox", { name: "开放申请", exact: true }).check();
  const request = page.waitForRequest(
    (r) => r.url().endsWith("/welfare/commands") && r.method() === "POST",
  );
  await modal.getByRole("button", { name: "保存", exact: true }).click();
  expect((await request).postDataJSON().config).toMatchObject({
    enabled: true,
    effective_at: "",
  });
});

test("旗舰 ISK 审批金额只读并通过发放合同结算", async ({ page }, info) => {
  await setup(page);
  const contract = {
    id: "902",
    owner_kind: "corporation",
    owner_id: "10",
    type: "item_exchange",
    status: "outstanding",
    checked_at: "2026-09-20T01:00:00Z",
    title: "旗舰补贴",
    issuer_id: "456",
    issuer_name: "荣耀海军",
    issuer_corporation_id: "10",
    for_corporation: true,
    assignee_id: "123",
    acceptor_id: "0",
    price: "0",
    reward: "1000000000.00",
    issued: "2026-09-20T00:00:00Z",
    completed: "",
    items_ready: true,
    content_token: "payout",
    items: [],
  };
  const item = {
    id: "1",
    account_id: member,
    corporation_id: "10",
    kind: "supercarrier",
    state: "submitted",
    version: "1",
    detail: {
      ...detail,
      base_minor: 1000000000000,
      purchase: { ...contract, id: "901", price: "10000000000", reward: "0" },
    },
    award_minor: 0,
    created_at: "2026-09-19T01:00:00Z",
    updated_at: "2026-09-19T01:00:00Z",
  };
  await page.route("**/api/v1/welfare/cases?**", (r) =>
    r.fulfill({ json: { data: { items: [item], next_cursor: "" } } }),
  );
  await page.route("**/api/v1/welfare/cases/1", (r) =>
    r.fulfill({ json: { data: { item, history: [] } } }),
  );
  await page.route("**/api/v1/welfare/cases/1/contracts**", (r) =>
    r.fulfill({
      json: { data: { items: [{ contract, can_link: true, reason: "" }] } },
    }),
  );
  await page.route("**/api/v1/welfare/commands", (r) => {
    item.state = "approved";
    item.award_minor = 100000000000;
    item.version = "2";
    return r.fulfill({ json: { data: item } });
  });
  await page.getByRole("button", { name: "旗舰补贴", exact: true }).click();
  await page.getByRole("button", { name: /测试成员 · #1/ }).click();
  await page.getByRole("button", { name: "批准", exact: true }).click();
  await expect(page.getByLabel("核准计价基数 / ISK")).toHaveCount(0);
  await page
    .getByRole("dialog")
    .getByRole("textbox")
    .fill("已核对购舰与历史资格");
  await page.getByRole("button", { name: "确认", exact: true }).click();
  await page.getByRole("button", { name: /测试成员 · #1/ }).click();
  await expect(
    page.getByRole("button", { name: "确认已交付", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "关联交付合同", exact: true }),
  ).toHaveCount(0);
  await expect(page.getByLabel("支付金额 / ISK", { exact: true })).toHaveValue(
    "1000000000",
  );
  await expect(page.getByText("等待合同同步", { exact: true })).toBeVisible();
  await page.screenshot({
    path: `../docs/ui/reviews/welfare/capital-delivery-${info.project.name}.png`,
    fullPage: true,
  });
  expect(
    await page.evaluate(
      () =>
        document.documentElement.scrollWidth <=
        document.documentElement.clientWidth,
    ),
  ).toBe(true);
});

test("PVP 核价缺失禁止按小计批准，可刷新核价或明确人工核价", async ({
  page,
}, info) => {
  await setup(page);
  const appraisal = {
    lines: [
      {
        input: "毒蜥级",
        name: "毒蜥级",
        type_id: "17715",
        quantity: "1",
        status: "ready",
        buy: "900.00",
        mid: "1000.00",
        sell: "1100.00",
        observed_at: "2026-09-19T01:00:00Z",
      },
    ],
    totals: { buy: "900.00", mid: "1000.00", sell: "1100.00" },
    adjusted: { buy: "720.00", mid: "800.00", sell: "880.00" },
    ratio_bps: 8000,
    complete: true,
  };
  const item = {
    id: "1",
    account_id: member,
    corporation_id: "10",
    kind: "solo",
    state: "submitted",
    version: "1",
    detail: {
      ...detail,
      contract_id: "0",
      valuation: {
        source: "market",
        state: "incomplete",
        reason: "部分物品缺少双边报价，不能按小计核准",
        at: "2026-09-19T01:00:00Z",
        amount_minor: 0,
        settings_version: "1",
        market: { ...appraisal, complete: false },
      },
    },
    award_minor: 0,
    created_at: "2026-09-19T01:00:00Z",
    updated_at: "2026-09-19T01:00:00Z",
  };
  await page.route("**/api/v1/welfare/cases?**", (r) =>
    r.fulfill({ json: { data: { items: [item], next_cursor: "" } } }),
  );
  await page.route("**/api/v1/welfare/cases/1", (r) =>
    r.fulfill({ json: { data: { item, history: [] } } }),
  );
  const bodies: Record<string, unknown>[] = [];
  await page.route("**/api/v1/welfare/commands", async (r) => {
    const body = r.request().postDataJSON();
    bodies.push(body);
    expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
    if (body.action === "appraise") {
      item.version = "2";
      item.detail.valuation = {
        ...item.detail.valuation,
        state: "ready",
        reason: "",
        amount_minor: 80000,
        market: appraisal,
      };
    }
    await r.fulfill({ json: { data: item } });
  });
  await page.getByRole("combobox", { name: "福利类型", exact: true }).click();
  await page.getByRole("option", { name: "PVP补损", exact: true }).click();
  await page.getByRole("button", { name: /测试成员 · #1/ }).click();
  await expect(
    page.getByText("部分物品缺少双边报价，不能按小计核准"),
  ).toBeVisible();
  await page.getByRole("button", { name: "批准", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "确认", exact: true }),
  ).toBeDisabled();
  await page.getByRole("checkbox", { name: "人工核价", exact: true }).check();
  await page.getByLabel("核价金额 / ISK").fill("900");
  await expect(page.getByLabel("人工核价原因")).toBeVisible();
  await page.getByRole("button", { name: "取消", exact: true }).click();
  await page.getByRole("button", { name: "重新核价", exact: true }).click();
  await expect(page.getByText("800 ISK", { exact: true }).first()).toBeVisible();
  await page.getByText("船体及全部物品 · 1 项", { exact: true }).click();
  mkdirSync("../docs/ui/reviews/welfare", { recursive: true });
  for (const width of info.project.name === "mobile" ? [375, 320] : [1280]) {
    await page.setViewportSize({ width, height: 900 });
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBeLessThanOrEqual(width);
    await page.screenshot({
      path: `../docs/ui/reviews/welfare/valuation-${width}.png`,
    });
  }
  await page.getByRole("button", { name: "批准", exact: true }).click();
  // The earlier explicit manual choice remains; switch back to the stored automatic quote.
  await page.getByRole("checkbox", { name: "人工核价", exact: true }).uncheck();
  await expect(page.getByLabel("核价金额 / ISK")).toHaveCount(0);
  await page.getByLabel("处理说明").fill("已核对船体及全部物品");
  await page.getByRole("button", { name: "确认", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(bodies.at(-1)).toMatchObject({
    action: "approve",
    version: "2",
    manual_pricing: false,
    detail: { base_minor: 80000 },
  });
});
test("旗舰自动发放显示合同复制字段和状态，窄屏不溢出", async ({
  page,
}, info) => {
  await setup(page);
  const item = {
    id: "1",
    account_id: member,
    corporation_id: "10",
    kind: "supercarrier",
    state: "approved",
    version: "3",
    reference: "WF-20260921-12345678-1234-4234-8234-123456789012",
    detail: { ...detail, payment_status: "waiting_contract" },
    award_minor: 50000000000,
    created_at: "2026-09-16T01:00:00Z",
    updated_at: "2026-09-16T01:00:00Z",
  };
  await page.route("**/api/v1/welfare/cases/1", (r) =>
    r.fulfill({ json: { data: { item, history: [] } } }),
  );
  await page.getByRole("button", { name: /测试成员 · #1/ }).click();
  await expect(page.getByLabel("支付金额 / ISK", { exact: true })).toHaveValue(
    "500000000",
  );
  await expect(page.getByLabel("合同结算 ID", { exact: true })).toHaveValue(
    item.reference,
  );
  await expect(page.getByText("等待合同同步", { exact: true })).toBeVisible();
  for (const name of ["关联交付合同", "领取交付任务", "确认已交付", "撤销批准"])
    await expect(page.getByRole("button", { name, exact: true })).toHaveCount(
      0,
    );
  for (const width of [1280, 375, 320]) {
    await page.setViewportSize({ width, height: 900 });
    expect(
      await page.evaluate(
        () =>
          document.documentElement.scrollWidth <=
          document.documentElement.clientWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path:
        "../docs/ui/reviews/welfare/unified-capital-" +
        info.project.name +
        "-" +
        width +
        ".png",
      fullPage: true,
    });
  }
});
test("补损批准后可复制合同信息，未批准与本人记录不显示", async ({
  page,
}, info) => {
  await setup(page);
  const item = {
    id: "1",
    reference: "PVP-20260921-A2C49E70-91F6-4AF1-8B52-3D8AE61C2097",
    account_id: member,
    corporation_id: "10",
    kind: "solo",
    state: "approved",
    version: "2",
    detail: { ...detail, base_minor: 5352666651 },
    award_minor: 5352666651,
    created_at: "2026-09-16T01:00:00Z",
    updated_at: "2026-09-16T01:00:00Z",
  };
  await page.route("**/api/v1/welfare/cases?**", (r) =>
    r.fulfill({ json: { data: { items: [item], next_cursor: "" } } }),
  );
  await page.route("**/api/v1/welfare/cases/1", (r) =>
    r.fulfill({ json: { data: { item, history: [] } } }),
  );
  await page.addInitScript(() => {
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: {
        writeText: async (value: string) => {
          if (sessionStorage.getItem("fail-copy")) throw new Error("denied");
          sessionStorage.setItem("copied-contract", value);
        },
      },
    });
  });
  await page.reload();
  await page.getByRole("button", { name: /测试成员 · #1/ }).click();
  const modal = page.getByRole("dialog");
  await expect(modal.getByLabel("接收角色", { exact: true })).toHaveValue(
    "测试成员",
  );
  await expect(modal.getByLabel("合同结算 ID", { exact: true })).toHaveValue(
    "PVP-20260921-A2C49E70-91F6-4AF1-8B52-3D8AE61C2097",
  );
  for (const label of [
    "领取交付任务",
    "撤销批准",
    "关联交付合同",
    "确认已交付",
  ]) {
    await expect(
      modal.getByRole("button", { name: label, exact: true }),
    ).toHaveCount(0);
  }
  await expect(modal.getByText("等待合同同步", { exact: true })).toBeVisible();
  await expect(
    modal
      .getByRole("region", { name: "合同信息", exact: true })
      .getByLabel("支付金额 / ISK", { exact: true }),
  ).toHaveValue("53526666");
  await modal
    .getByRole("button", { name: "复制支付金额", exact: true })
    .click();
  expect(
    await page.evaluate(() => sessionStorage.getItem("copied-contract")),
  ).toBe("53526666");
  await modal
    .getByRole("button", { name: "复制接收角色", exact: true })
    .click();
  expect(
    await page.evaluate(() => sessionStorage.getItem("copied-contract")),
  ).toBe("测试成员");
  await modal
    .getByRole("button", { name: "复制合同结算 ID", exact: true })
    .click();
  expect(
    await page.evaluate(() => sessionStorage.getItem("copied-contract")),
  ).toBe("PVP-20260921-A2C49E70-91F6-4AF1-8B52-3D8AE61C2097");
  await page.evaluate(() => sessionStorage.setItem("fail-copy", "yes"));
  await modal
    .getByRole("button", { name: "复制合同结算 ID", exact: true })
    .click();
  await expect(modal.getByRole("alert")).toContainText("复制失败，请手动复制");
  await page.setViewportSize({
    width: info.project.name.includes("mobile") ? 375 : 1280,
    height: 900,
  });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  mkdirSync("../docs/ui/reviews/welfare", { recursive: true });
  await page.screenshot({
    path: `../docs/ui/reviews/welfare/contract-copy-${info.project.name}.png`,
  });
  for (const state of ["submitted", "rejected", "completed"]) {
    item.state = state;
    await page.reload();
    await page.getByRole("button", { name: /测试成员 · #1/ }).click();
    await expect(
      page.getByRole("button", { name: "复制合同结算 ID", exact: true }),
    ).toHaveCount(0);
    await expect(
      page.getByRole("button", { name: "复制支付金额", exact: true }),
    ).toHaveCount(0);
  }
  item.state = "executing";
  item.kind = "srp";
  await page.reload();
  await page.getByRole("button", { name: /测试成员 · #1/ }).click();
  await expect(
    page.getByRole("button", { name: "复制合同结算 ID", exact: true }),
  ).toBeVisible();
  item.account_id = user;
  await page.reload();
  await page.getByRole("button", { name: /测试成员 · #1/ }).click();
  await expect(
    page.getByRole("button", { name: "复制合同结算 ID", exact: true }),
  ).toHaveCount(0);
});

test("补损上限与比例可分别配置，批准显示折算金额", async ({ page }, info) => {
  await setup(page);
  const requests: any[] = [];
  await page.route("**/api/v1/welfare/commands", (r) => {
    requests.push(r.request().postDataJSON());
    return r.fulfill({ json: { data: { saved: true } } });
  });
  await page.getByRole("button", { name: "补损设置", exact: true }).click();
  await page.getByLabel("补损比例（%）", { exact: true }).fill("75.25");
  await page.getByLabel("单笔上限 / ISK", { exact: true }).fill("200000000");
  await page.getByLabel("每人每日上限 / ISK").fill("1000000");
  await page.getByLabel("每人每周上限 / ISK").fill("5000000");
  await page.getByLabel("每人每月上限 / ISK").fill("10000000");
  mkdirSync("../docs/ui/reviews/welfare", { recursive: true });
  await page.screenshot({
    path: `../docs/ui/reviews/welfare/loss-quota-${info.project.name}.png`,
    fullPage: true,
  });
  await page.getByRole("button", { name: "保存", exact: true }).click();
  expect(requests[0]).toMatchObject({
    action: "configure",
    kind: "srp",
    version: "1",
    config: {
      loss_rate_bps: 7525,
      loss_cap_minor: 20000000000,
      loss_daily_cap_minor: 100000000,
      loss_weekly_cap_minor: 500000000,
      loss_monthly_cap_minor: 1000000000,
    },
  });
  await page.getByRole("combobox", { name: "福利类型", exact: true }).click();
  await page.getByRole("option", { name: "PVP补损", exact: true }).click();
  await page.getByRole("button", { name: "补损设置", exact: true }).click();
  await expect(page.getByLabel("补损比例（%）", { exact: true })).toHaveValue(
    "100",
  );
  await expect(page.getByLabel("单笔上限 / ISK", { exact: true })).toHaveValue(
    "",
  );
  await expect(page.getByLabel("每人每日上限 / ISK")).toHaveValue("");
  await expect(page.getByLabel("每人每周上限 / ISK")).toHaveValue("");
  await expect(page.getByLabel("每人每月上限 / ISK")).toHaveValue("");
  await page.getByRole("button", { name: "保存", exact: true }).click();
  expect(requests[1]).toMatchObject({
    action: "configure",
    kind: "solo",
    config: {
      loss_rate_bps: 10000,
      loss_cap_minor: 0,
      loss_daily_cap_minor: 0,
      loss_weekly_cap_minor: 0,
      loss_monthly_cap_minor: 0,
    },
  });
  await page.getByRole("combobox", { name: "福利类型", exact: true }).click();
  await page.getByRole("option", { name: "军团补损", exact: true }).click();
  await page.getByRole("button", { name: /测试成员 · #1/ }).click();
  await page.getByRole("button", { name: "批准", exact: true }).click();
  await page.getByLabel("核价金额 / ISK").fill("");
  await expect(
    page.getByRole("button", { name: "确认", exact: true }),
  ).toBeVisible();
  await page.getByLabel("核价金额 / ISK").fill("1000");
  await expect(
    page.getByText("核准金额 1,000 ISK", { exact: true }),
  ).toBeVisible();
  await page.getByLabel("处理说明").fill("核价通过");
  await page.getByRole("button", { name: "确认", exact: true }).click();
  expect(requests.at(-1)).toMatchObject({
    action: "approve",
    loss_policy_version: "1",
    detail: { base_minor: 100000 },
  });
});

async function setup(page: Page, admin = true) {
  await page.route("https://images.evetech.net/**", (r) => r.abort());
  await page.route("**/api/v1/**", (r) => {
    const url = new URL(r.request().url()),
      p = url.pathname;
    let data: unknown = {};
    if (p === "/api/v1/modules")
      data = [
        "system",
        "identity",
        "eve",
        "access",
        "exchange",
        "welfare",
        "fittings",
        "skills",
      ].map((id) => ({ id, api_version: 1, version: "0.1.0" }));
    else if (p === "/api/v1/identity/session")
      data = {
        authenticated: true,
        session: {
          user_id: admin ? user : member,
          character: { id: "123", name: "测试成员" },
          csrf_token: "csrf",
          expires_at: "2099-01-01T00:00:00Z",
        },
      };
    else if (p === "/api/v1/access/me")
      data = {
        administrator: admin,
        can_manage: admin,
        can_manage_sync: false,
        site_roles: [],
        characters: [],
      };
    else if (p === "/api/v1/welfare/context")
      data = {
        corporations: [{ id: "10", name: "荣耀海军", can_manage: admin }],
        characters: [
          {
            id: "123",
            name: "测试成员",
            account_id: admin ? user : member,
            corporation_id: "10",
          },
        ],
        administrator: admin,
        policies: ["srp", "growth_gila", "capital"].map((kind) => ({
          corporation_id: "10",
          kind,
          version: "1",
          config,
        })),
      };
    else if (p === "/api/v1/welfare/growth/check")
      data = {
        state: "met",
        remaining_sp: 0,
        observed_at: "2026-09-20T01:00:00Z",
      };
    else if (p === "/api/v1/welfare/cases")
      data = {
        items:
          url.searchParams.get("kind") === "srp"
            ? [
                {
                  id: "1",
                  account_id: member,
                  corporation_id: "10",
                  kind: "srp",
                  state: "submitted",
                  version: "1",
                  detail,
                  award_minor: 0,
                  created_at: "2026-09-16T01:00:00Z",
                  updated_at: "2026-09-16T01:00:00Z",
                },
              ]
            : [],
        next_cursor: "",
      };
    else if (p === "/api/v1/welfare/losses")
      data = { items: [syncedLoss], next_cursor: "" };
    else if (p === "/api/v1/welfare/losses/prices")
      data = { items: ["1000.00", "2000.00", "3000.00", "4000.00", "5000.00", "10000.00", "120.00"].map((mid) => ({ mid, observed_at: "2026-09-16T01:00:00Z" })) };
    else if (p === "/api/v1/welfare/cases/1")
      data = {
        item: {
          id: "1",
          account_id: member,
          corporation_id: "10",
          kind: "srp",
          state: "submitted",
          version: "1",
          detail,
          award_minor: 0,
          created_at: "2026-09-16T01:00:00Z",
          updated_at: "2026-09-16T01:00:00Z",
        },
        history: [
          { action: "apply", note: "", created_at: "2026-09-16T01:00:00Z" },
        ],
      };
    else if (p === "/api/v1/welfare/members")
      data = {
        items: [
          {
            id: "123",
            name: "测试成员",
            account_id: member,
            corporation_id: "10",
          },
          {
            id: "124",
            name: "同一成员小号",
            account_id: member,
            corporation_id: "10",
          },
        ],
      };
    else if (p === "/api/v1/welfare/preview") {
      const body = r.request().postDataJSON();
      expect(body.lines).toEqual([{ account_id: member, amount_minor: 1250 }]);
      data = { token: "preview", lines: body.lines, total_minor: 1250 };
    } else if (p === "/api/v1/welfare/commands") {
      expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
      const body = r.request().postDataJSON();
      if (body.action === "grant") {
        expect(body.token).toBe("preview");
        expect(body.lines).toHaveLength(1);
      }
      data = { saved: true };
    } else if (p === "/api/v1/fittings/library")
      data = [
        { id: "1", name: "毒蜥标准配装", fit: { ship_type_id: "17715" } },
      ];
    else if (p === "/api/v1/skills/plans")
      data = [{ id: "1", name: "毒蜥技能要求" }];
    else if (p === "/api/v1/exchange/catalog")
      data = { items: [{ id: "7", version: "1", name: "摄影赛礼包", archived: false, content: { fittings: [], items: [{ type_id: "34", quantity: 10, name: "三钛合金" }] } }], next_cursor: "" };
    return r.fulfill({ json: { data } });
  });
  await page.goto("/welfare");
  await expect(
    page.getByRole("heading", { name: "军团福利", exact: true }),
  ).toBeVisible();
}
test("活动福利按项目申请并提交截图，成员看不到项目配置", async ({ page }, info) => {
  await setup(page, false);
  await page.route("**/api/v1/welfare/context**", (route) => route.fulfill({ json: { data: {
    corporations: [{ id: "10", name: "荣耀海军", can_manage: false }],
    characters: [{ id: "123", name: "测试成员", account_id: member, corporation_id: "10" }],
    administrator: false,
    policies: [{ corporation_id: "10", kind: "activity_1", version: "1", config: { enabled: true, project_name: "舰队摄影赛", reward_id: "7", reward_version: "1", rewards: { fittings: [], items: [{ type_id: "34", quantity: 10, name: "三钛合金" }], coins_minor: 125 } } }],
  } } }));
  await page.reload();
  await page.getByRole("button", { name: "活动福利", exact: true }).click();
  await expect(page.getByText("舰队摄影赛")).toBeVisible();
  await expect(page.getByRole("button", { name: "新增项目" })).toHaveCount(0);
  await page.screenshot({ path: `../docs/ui/reviews/welfare/activity-${info.project.name}.png`, fullPage: true });
  await page.getByRole("button", { name: "申请", exact: true }).click();
  const modal = page.getByRole("dialog");
  await page.screenshot({ path: `../docs/ui/reviews/welfare/activity-application-${info.project.name}.png`, fullPage: true });
  await expect(modal.getByRole("button", { name: "提交申请" })).toBeDisabled();
  await modal.getByLabel("活动说明").fill("参与舰队摄影赛，提交活动截图");
  const png = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y3cpxsAAAAASUVORK5CYII=", "base64");
  await modal.locator('input[type="file"]').setInputFiles({ name: "fleet.png", mimeType: "image/png", buffer: png });
  await expect(modal.getByRole("button", { name: "提交申请" })).toBeEnabled();
  let uploaded = false;
  await page.route("**/api/v1/welfare/activity/applications", (route) => {
    const body = route.request().postDataBuffer()?.toString() || "";
    expect(body).toContain("project_id");
    expect(body).toContain("fleet.png");
    uploaded = true;
    return route.fulfill({ json: { data: { id: "8" } } });
  });
  await modal.getByRole("button", { name: "提交申请" }).click();
  await expect(modal).toHaveCount(0);
  expect(uploaded).toBe(true);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test("活动福利项目编辑弹窗布局", async ({ page }, info) => {
  await setup(page);
  await page.route("**/api/v1/welfare/context**", (route) => route.fulfill({ json: { data: {
    corporations: [{ id: "10", name: "荣耀海军", can_manage: true }],
    characters: [{ id: "123", name: "测试成员", account_id: user, corporation_id: "10" }],
    administrator: true,
    policies: [{ corporation_id: "10", kind: "activity_1", version: "1", config: { enabled: true, project_name: "舰队摄影赛", reward_id: "7", reward_version: "1", rewards: { fittings: [], items: [{ type_id: "34", quantity: 10, name: "三钛合金" }], coins_minor: 125 } } }],
  } } }));
  await page.reload();
  await page.getByRole("button", { name: "活动福利", exact: true }).click();
  await page.getByRole("button", { name: "编辑舰队摄影赛" }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await expect(page.getByRole("dialog").getByText("三钛合金")).toBeVisible();
  await expect(page.getByRole("dialog").getByRole("combobox", { name: "奖励库" })).toContainText("摄影赛礼包");
  await page.screenshot({ path: `../docs/ui/reviews/welfare/activity-project-${info.project.name}.png`, fullPage: true });
  const saved = page.waitForRequest("**/api/v1/welfare/activity/projects");
  await page.getByRole("dialog").getByRole("button", { name: "保存" }).click();
  expect((await saved).postDataJSON()).toMatchObject({ reward_id: "7", reward_version: "1", coins_minor: 125 });
});
test("English welfare layout", async ({ page }, info) => {
  await setup(page);
  await englishLayout(
    page,
    "Corporation welfare",
    `welfare-${info.project.name}`,
  );
  for (const name of ["Growth benefits", "Capital subsidies"]) {
    await page.getByRole("button", { name, exact: true }).click();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
  }
});

test("四项入口与紧凑列表，移动端不溢出", async ({ page }, info) => {
  await setup(page);
  for (const name of ["成长福利", "旗舰补贴", "果壳币", "补损"])
    await page.getByRole("button", { name, exact: true }).click();
  await expect(
    page.getByRole("button", { name: /测试成员 · #1/ }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  mkdirSync("../docs/ui/reviews/welfare", { recursive: true });
  await page.screenshot({
    path: `../docs/ui/reviews/welfare/${info.project.name}.png`,
    fullPage: true,
  });
});
test("主动发币必须先预览，成员按账号去重", async ({ page }) => {
  await setup(page);
  await page.getByRole("button", { name: "果壳币", exact: true }).click();
  await page.getByRole("button", { name: "发放果壳币", exact: true }).click();
  const modal = page.getByRole("dialog");
  await modal.getByRole("combobox", { name: "领取成员 1" }).click();
  await expect(page.getByRole("option")).toHaveCount(1);
  await page.getByRole("option").click();
  await modal.getByLabel("币数", { exact: true }).fill("12.50");
  await modal.getByLabel("发放原因").fill("后勤贡献奖励");
  await modal.getByRole("button", { name: "预览发放" }).click();
  await expect(modal.getByText("12.5", { exact: true }).first()).toBeVisible();
  await modal.getByRole("button", { name: "确认发放" }).click();
  await expect(modal).toHaveCount(0);
});
test("普通成员隐藏管理操作，统一申请弹窗可关闭", async ({ page }) => {
  await setup(page, false);
  await expect(
    page.getByRole("button", { name: "手动录入", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "申请", exact: true }),
  ).toHaveCount(0);
  await expect(page.getByRole("button", { name: "福利规则" })).toHaveCount(0);
  await page.getByRole("button", { name: "成长福利", exact: true }).click();
  await page.getByRole("button", { name: "申请", exact: true }).click();
  await expect(page.getByRole("dialog", { name: "申请毒蜥" })).toBeVisible();
  await expect(
    page.getByRole("dialog").getByRole("button", { name: "关闭", exact: true }),
  ).toBeFocused();
  await expect(page.getByLabel("用途 / 拟参加活动")).toHaveCount(0);
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "提交申请" })
    .focus();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.getByRole("button", { name: "果壳币", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "发放果壳币", exact: true }),
  ).toHaveCount(0);
});
test("管理员可核价，业务操作不使用原生确认框", async ({ page }) => {
  await setup(page);
  await expect(
    page.getByRole("button", { name: "申请", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "手动录入", exact: true }).click();
  await expect(
    page.getByRole("dialog", { name: "手动录入 · 军团补损" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "提交录入", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "关闭", exact: true })
    .click();
  await page.getByRole("button", { name: /测试成员 · #1/ }).click();
  await page.getByRole("button", { name: "批准", exact: true }).click();
  await page.getByLabel("核价金额 / ISK").fill("500000000");
  await page.getByLabel("处理说明").fill("已核对购舰合同及集结记录");
  await page.getByRole("button", { name: "确认", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
});
test("同步损失可查看星系物品并带入补损，手机不溢出", async ({ page }, info) => {
  await setup(page, false);
  await expect(
    page.getByRole("button", { name: "舰船损失", exact: true }),
  ).toHaveCount(0);
  await navigate(page, "舰船损失", "作战与训练");
  await expect(page).toHaveURL(/\/losses$/);
  await expect(
    page.getByRole("heading", { name: "舰船损失", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  const browser = page.getByRole("region", { name: "损失记录", exact: true });
  await expect(browser.getByText("可申请补损", { exact: true })).toBeVisible();
  const lossRow = browser.getByRole("button", { name: /毒蜥级/ });
  await expect(lossRow.getByText("KM #987", { exact: true })).toBeVisible();
  await expect(lossRow.locator(".eve-image")).toHaveCount(1);
  await page.screenshot({
    path: `../docs/ui/reviews/welfare/losses-list-${info.project.name}.png`,
    fullPage: true,
  });
  await lossRow.click();
  await expect(browser.getByText("吉他", { exact: true })).toBeVisible();
  await expect(browser.getByRole("region", { name: "参与者" })).toContainText("最后一击角色");
  await expect(browser.getByRole("region", { name: "参与者" })).toContainText("承受伤害 8,000");
  await expect(browser.locator(".welfare-km-attacker-damage")).toHaveText([/5,000\s*62\.5%/, /3,000\s*37\.5%/]);
  await expect(browser.locator(".welfare-km-damage-track")).toHaveCount(0);
  for (const group of ["高能量槽", "中能量槽", "低能量槽", "改装件安装座", "无人机挂舱", "货柜舱"]) {
    await expect(browser.getByText(group, { exact: true })).toBeVisible();
  }
  await expect(browser.getByText("三钛合金", { exact: true }).first()).toBeVisible();
  const itemRow = browser.locator(".welfare-km-items .welfare-km-item-row:not(.welfare-columns)").first();
  const cargoRows = browser.locator(".welfare-km-item-nested").filter({ hasText: "三钛合金" });
  await expect(cargoRows).toHaveCount(2);
  await expect(cargoRows.filter({ hasText: "掉落" }).locator(".welfare-km-item-price")).toHaveText("72");
  await expect(cargoRows.filter({ hasText: "损毁" }).locator(".welfare-km-item-price")).toHaveText("48");
  expect(await itemRow.evaluate((row) => {
    const bounds = row.getBoundingClientRect();
    return Array.from(row.querySelectorAll(":scope > span")).slice(1).every((cell) => {
      const rect = cell.getBoundingClientRect();
      return rect.width > 0 && rect.right <= bounds.right + 1;
    });
  })).toBe(true);
  if (info.project.name === "desktop") {
    await expect(browser.locator(".welfare-km-items .welfare-columns").getByText("数量")).toBeVisible();
    await expect(browser.locator(".welfare-km-items .welfare-columns").getByText("参考价")).toBeVisible();
  }
  expect(await browser.evaluate((e) => e.scrollWidth <= e.clientWidth)).toBe(
    true,
  );
  await page.screenshot({
    path: `../docs/ui/reviews/welfare/losses-${info.project.name}.png`,
    fullPage: true,
  });
  if (info.project.name === "desktop") {
    for (const width of [375, 320]) {
      await page.setViewportSize({ width, height: 900 });
      await expect(itemRow.locator(".welfare-km-item-quantity")).toBeVisible();
      await expect(itemRow.locator(".welfare-km-item-price")).toBeVisible();
      expect(
        await browser.evaluate((e) => e.scrollWidth <= e.clientWidth),
      ).toBe(true);
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
      ).toBe(true);
      await page.screenshot({
        path: `../docs/ui/reviews/welfare/losses-${width}.png`,
      });
    }
  }
  await browser.getByRole("button", { name: "申请补损", exact: true }).click();
  const form = page.getByRole("dialog", { name: "申请军团补损", exact: true });
  await expect(form.getByText("吉他", { exact: true })).toBeVisible();
  await expect(form.getByLabel("击毁报告 ID", { exact: true })).toHaveCount(0);
  await form.getByLabel("情况说明").fill("集结行动损失");
  const posted = page.waitForRequest((r) =>
    r.url().endsWith("/welfare/commands"),
  );
  await form.getByRole("button", { name: "提交申请" }).click();
  expect((await posted).postDataJSON().detail).toMatchObject({
    synced_loss: true,
    character_id: "123",
    killmail_id: "987",
    event_id: "1",
    ship_type_id: "17715",
    occurred_at: syncedLoss.occurred_at,
  });
  await expect(form).toHaveCount(0);
  await expect(page.getByText("补损申请已提交", { exact: true })).toBeVisible();
  await expect(page).toHaveURL(/\/losses$/);
});

test("旧击毁报告缺参与者时明确提示待同步", async ({ page }) => {
  await setup(page, false);
  const legacyLoss = { ...syncedLoss } as Partial<typeof syncedLoss>;
  delete legacyLoss.attackers;
  await page.route("**/api/v1/welfare/losses?**", (route) =>
    route.fulfill({ json: { data: { items: [legacyLoss], next_cursor: "" } } }),
  );
  await navigate(page, "舰船损失", "作战与训练");
  const browser = page.getByRole("region", { name: "损失记录", exact: true });
  await browser.getByRole("button", { name: /毒蜥级/ }).click();
  await expect(browser.getByRole("region", { name: "参与者" })).toContainText("参与者资料尚未同步");
  await expect(browser.getByText("三钛合金", { exact: true }).first()).toBeVisible();
});
test("反复切换福利与损失页面并关闭下拉后仍可操作", async ({ page }) => {
  test.setTimeout(90000);
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await setup(page);
  for (let round = 0; round < 30; round++) {
    await navigate(page, "舰船损失", "作战与训练");
    if (round % 5 === 0) {
      await page
        .getByRole("combobox", { name: "损失角色", exact: true })
        .click();
      await page.keyboard.press("Escape");
      await expect(page.getByRole("listbox")).toHaveCount(0);
    }
    await navigate(page, "军团福利", "福利与兑换");
  }
  await expect(
    page.getByRole("heading", { name: "军团福利", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "手动录入", exact: true }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "关闭", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(page.locator("body")).not.toHaveCSS("pointer-events", "none");
  expect(errors).toEqual([]);
});

test("未关联出勤的损失仅允许 PVP 补损", async ({ page }, info) => {
  await setup(page, false);
  await page.route("**/api/v1/welfare/losses?**", (r) =>
    r.fulfill({
      json: {
        data: {
          items: [
            {
              ...syncedLoss,
              reimbursement: {
                status: "available",
                state: "",
                kind: "",
                reason: "attendance_unlinked",
                attendance_event_id: "0",
                available_kinds: ["solo"],
              },
            },
          ],
          next_cursor: "",
        },
      },
    }),
  );
  await page.goto("/losses");
  const region = page.getByRole("region", { name: "损失记录", exact: true });
  await expect(
    region.getByText("可申请PVP补损", { exact: true }),
  ).toBeVisible();
  await region.getByRole("button", { name: /毒蜥级/ }).click();
  await expect(
    region.getByText("未关联有效出勤，无法申请军团补损", { exact: true }),
  ).toHaveCount(0);
  await page.getByRole("combobox", { name: "补损类型" }).click();
  await expect(page.getByRole("option")).toHaveCount(1);
  await page.getByRole("option", { name: "PVP补损", exact: true }).click();
  await page.screenshot({
    path: `../docs/ui/reviews/welfare/unlinked-attendance-${info.project.name}.png`,
    fullPage: true,
  });
  await region.getByRole("button", { name: "申请补损", exact: true }).click();
  await expect(
    page.getByRole("dialog", { name: "申请PVP补损", exact: true }),
  ).toBeVisible();
});

test("损失同步错误可见，重试不会隐藏已有记录", async ({ page }) => {
  await setup(page, false);
  await page.route("**/api/v1/eve/sync/characters/123/refresh", (r) =>
    r.fulfill({
      status: 409,
      json: { error: { message: "请重新授权击毁报告" } },
    }),
  );
  await navigate(page, "舰船损失", "作战与训练");
  const modal = page.getByRole("region", { name: "损失记录", exact: true });
  await modal.getByRole("button", { name: "同步击毁记录" }).click();
  await expect(modal.getByRole("alert")).toHaveText("请重新授权击毁报告");
  await expect(modal.getByRole("button", { name: /毒蜥级/ })).toBeVisible();
});

test("损失补偿状态区分审核和交付，隐藏重复申请", async ({ page }) => {
  await setup(page, false);
  for (const [status, state, label] of [
    ["processing", "approved", "待交付"],
    ["completed", "completed", "已补损"],
  ]) {
    await page.route("**/api/v1/welfare/losses?**", (r) =>
      r.fulfill({
        json: {
          data: {
            items: [
              {
                ...syncedLoss,
                reimbursement: {
                  status,
                  state,
                  kind: "srp",
                  available_kinds: [],
                },
              },
            ],
            next_cursor: "",
          },
        },
      }),
    );
    await page.goto("/losses");
    const region = page.getByRole("region", { name: "损失记录", exact: true });
    await expect(region.getByText(label, { exact: true })).toBeVisible();
    await region.getByRole("button", { name: /毒蜥级/ }).click();
    await expect(region.getByText(label, { exact: true })).toBeVisible();
    await expect(
      region.getByRole("button", { name: "申请补损", exact: true }),
    ).toHaveCount(0);
  }
});

test("无需配置规则即可提交，损失页面无额外资格入口", async ({ page }) => {
  await setup(page, false);
  await page.route("**/api/v1/welfare/context**", (r) =>
    r.fulfill({
      json: {
        data: {
          administrator: false,
          corporations: [{ id: "10", name: "荣耀海军", can_manage: false }],
          characters: [
            {
              id: "123",
              name: "测试成员",
              account_id: member,
              corporation_id: "10",
            },
          ],
          policies: [],
        },
      },
    }),
  );
  await page.goto("/losses");
  await page.getByRole("button", { name: /毒蜥级/ }).click();
  await page.getByRole("combobox", { name: "补损类型" }).click();
  await expect(page.getByRole("option")).toHaveCount(2);
  await expect(
    page.getByRole("option", { name: "联盟补损", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("option", { name: "PVP补损", exact: true }).click();
  await page.getByRole("button", { name: "申请补损", exact: true }).click();
  const modal = page.getByRole("dialog", { name: "申请PVP补损", exact: true });
  await expect(modal.getByText(/规则尚未生效/)).toHaveCount(0);
  await modal.getByLabel("情况说明").fill("提交人工核验");
  await modal.getByRole("button", { name: "提交申请", exact: true }).click();
  await expect(modal).toHaveCount(0);
});

test("管理员损失页无规则资格入口，成长旗舰资格仍按业务分组", async ({
  page,
}, info) => {
  await setup(page);
  const profile = {
    account_id: member,
    version: "2",
    verified: true,
    history: { growth_gila: "used", supercarrier: "unused", titan: "unknown" },
    months: ["2026-08"],
  };
  await page.route("**/api/v1/welfare/profile?**", (r) =>
    r.fulfill({ json: { data: profile } }),
  );
  for (const [tab, title, count] of [["旗舰补贴", "旗舰资格", 2]] as const) {
    await page.getByRole("button", { name: tab, exact: true }).click();
    await page.getByRole("button", { name: title, exact: true }).click();
    const modal = page.getByRole("dialog", { name: title, exact: true });
    await expect(modal.getByRole("combobox", { name: /历史资格/ })).toHaveCount(
      count,
    );
    await expect(
      modal.getByLabel("已确认 Active 月（YYYY-MM，逗号分隔）"),
    ).toHaveCount(0);
    await expect(modal.getByLabel("核验依据")).toHaveCount(0);
    const posted = page.waitForRequest((r) =>
      r.url().endsWith("/welfare/commands"),
    );
    await modal.getByRole("button", { name: "保存", exact: true }).click();
    expect((await posted).postDataJSON().profile).toMatchObject(profile);
    await expect(modal).toHaveCount(0);
    await page
      .getByRole("button", {
        name: tab === "成长福利" ? "项目配置" : tab + "规则",
        exact: true,
      })
      .click();
    const rule = page.getByRole("dialog");
    if (tab === "旗舰补贴") {
      await rule.getByRole("combobox", { name: "规则项目" }).click();
      await expect(
        page.getByRole("option", { name: "PVP补损", exact: true }),
      ).toHaveCount(0);
      await page.keyboard.press("Escape");
    } else {
      await expect(
        rule.getByRole("combobox", { name: "军团配装" }),
      ).toBeVisible();
    }
    await rule.getByRole("button", { name: "关闭", exact: true }).click();
  }
  await page.goto("/losses");
  await expect(
    page.getByRole("region", { name: "损失记录", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: /补损规则|补损资格/ }),
  ).toHaveCount(0);
  await page.screenshot({
    path:
      "../docs/ui/reviews/welfare/manual-review-" + info.project.name + ".png",
    fullPage: true,
  });
});

stateLayouts({
  name: "welfare",
  setup: setup,
  path: "/welfare",
  endpoint: "**/api/v1/welfare/cases?**",
  empty: { items: [], next_cursor: "" },
  emptyText: /暂无.*记录|No records/,
});

stateLayouts({
  name: "losses",
  setup: setup,
  path: "/losses",
  endpoint: "**/api/v1/welfare/context**",
  empty: {
    administrator: false,
    corporations: [],
    characters: [],
    policies: [],
  },
  emptyText: /暂无可用军团|No available corporations/,
});

test("补损取消申请保留原因并按状态显示入口", async ({ page }) => {
  await setup(page, false);
  const item = {
    id: "1",
    account_id: member,
    corporation_id: "10",
    kind: "srp",
    state: "approved",
    version: "2",
    detail,
    award_minor: 10000,
    created_at: "2026-09-21T01:00:00Z",
    updated_at: "2026-09-21T01:00:00Z",
  };
  await page.route("**/api/v1/welfare/cases?**", (r) =>
    r.fulfill({ json: { data: { items: [item], next_cursor: "" } } }),
  );
  await page.route("**/api/v1/welfare/cases/1", (r) =>
    r.fulfill({ json: { data: { item, history: [] } } }),
  );
  const writes: any[] = [];
  await page.route("**/api/v1/welfare/commands", (r) => {
    writes.push(r.request().postDataJSON());
    return r.fulfill({
      status: 409,
      json: { error: { code: "welfare_conflict", message: "记录已更新" } },
    });
  });
  await page.reload();
  await page.getByRole("button", { name: /测试成员 · #1/ }).click();
  await page.getByRole("button", { name: "申请取消", exact: true }).click();
  await page
    .getByRole("textbox", { name: "取消原因", exact: true })
    .fill("重复申请，请取消");
  await page.getByRole("button", { name: "确认", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("记录已更新");
  await expect(
    page.getByRole("textbox", { name: "取消原因", exact: true }),
  ).toHaveValue("重复申请，请取消");
  expect(writes[0]).toMatchObject({
    action: "request_cancel",
    id: "1",
    version: "2",
    note: "重复申请，请取消",
    confirmed_not_delivered: false,
  });
  for (const state of ["cancel_requested", "completed", "cancelled"]) {
    item.state = state;
    await page.reload();
    await page.getByRole("button", { name: /测试成员 · #1/ }).click();
    await expect(
      page.getByRole("button", { name: "申请取消", exact: true }),
    ).toHaveCount(0);
    await expect(
      page.getByRole("button", { name: "确认取消", exact: true }),
    ).toHaveCount(0);
  }
});

test("补损取消审核必须确认未付款，待取消隐藏合同复制", async ({
  page,
}, info) => {
  await setup(page);
  const item = {
    id: "1",
    account_id: member,
    corporation_id: "10",
    kind: "srp",
    state: "cancel_requested",
    version: "3",
    detail: {
      ...detail,
      cancellation: {
        reason: "重复申请，请取消",
        requested_at: "2026-09-21T01:00:00Z",
      },
    },
    award_minor: 10000,
    created_at: "2026-09-21T01:00:00Z",
    updated_at: "2026-09-21T01:00:00Z",
  };
  await page.route("**/api/v1/welfare/cases?**", (r) =>
    r.fulfill({ json: { data: { items: [item], next_cursor: "" } } }),
  );
  await page.route("**/api/v1/welfare/cases/1", (r) =>
    r.fulfill({ json: { data: { item, history: [] } } }),
  );
  const writes: any[] = [];
  await page.route("**/api/v1/welfare/commands", (r) => {
    writes.push(r.request().postDataJSON());
    return r.fulfill({ json: { data: item } });
  });
  await page.reload();
  await page.getByRole("button", { name: /测试成员 · #1/ }).click();
  await expect(
    page.getByRole("button", { name: "复制合同结算 ID", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByText("重复申请，请取消", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "确认取消", exact: true }).click();
  await page
    .getByLabel("处理说明", { exact: true })
    .fill("游戏中已核对，无付款合同");
  const submit = page.getByRole("button", { name: "确认", exact: true });
  await expect(submit).toBeDisabled();
  await page
    .getByRole("checkbox", {
      name: "已在游戏中核对：未发放，且相关发放合同已撤销",
    })
    .check();
  await expect(submit).toBeEnabled();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: `../docs/ui/reviews/welfare/cancellation-${info.project.name}.png`,
    fullPage: true,
  });
  await submit.click();
  expect(writes[0]).toMatchObject({
    action: "approve_cancel",
    version: "3",
    confirmed_not_delivered: true,
    note: "游戏中已核对，无付款合同",
  });
  await page.getByRole("button", { name: /测试成员 · #1/ }).click();
  await page.getByRole("button", { name: "驳回取消", exact: true }).click();
  await page.getByLabel("驳回原因", { exact: true }).fill("已创建发放合同");
  await page.getByRole("button", { name: "确认", exact: true }).click();
  expect(writes[1]).toMatchObject({
    action: "reject_cancel",
    note: "已创建发放合同",
    confirmed_not_delivered: false,
  });
});

test("成长福利实物自动发放不显示 ISK 和手动完成，历史纯币可确认", async ({
  page,
}, info) => {
  await setup(page);
  const item = {
    id: "1",
    account_id: member,
    corporation_id: "10",
    kind: "growth_fitting_1",
    state: "approved",
    version: "3",
    reference: "WF-20260921-12345678-1234-4234-8234-123456789012",
    detail: {
      ...detail,
      payment_status: "waiting_contract",
      rewards: {
        fittings: [],
        items: [{ type_id: "34", quantity: 100, name: "三钛合金" }],
        coins_minor: 125,
      },
    },
    award_minor: 0,
    created_at: "2026-09-16T01:00:00Z",
    updated_at: "2026-09-16T01:00:00Z",
  };
  await page.route("**/api/v1/welfare/cases/1", (r) =>
    r.fulfill({ json: { data: { item, history: [] } } }),
  );
  await page.getByRole("button", { name: /测试成员 · #1/ }).click();
  await expect(page.getByLabel("合同结算 ID", { exact: true })).toHaveValue(
    item.reference,
  );
  await expect(page.getByLabel("支付金额 / ISK", { exact: true })).toHaveCount(
    0,
  );
  await expect(
    page.getByRole("button", { name: "确认已交付", exact: true }),
  ).toHaveCount(0);
  await page.setViewportSize({ width: 375, height: 900 });
  expect(
    await page.evaluate(
      () =>
        document.documentElement.scrollWidth <=
        document.documentElement.clientWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path:
      "../docs/ui/reviews/welfare/unified-growth-" + info.project.name + ".png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "关闭", exact: true }).click();
  item.detail.rewards.items = [];
  item.detail.payment_status = "coins_review_required";
  await page.reload();
  await page.getByRole("button", { name: /测试成员 · #1/ }).click();
  await expect(
    page.getByRole("button", { name: "发放果壳币", exact: true }),
  ).toBeVisible();
  await expect(page.getByLabel("合同结算 ID", { exact: true })).toHaveCount(0);
});
