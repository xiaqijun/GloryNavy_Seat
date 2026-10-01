import { test, expect, type Page } from "@playwright/test";
import { mkdirSync } from "node:fs";
const admin = "00000000-0000-4000-8000-000000000001",
  member = "00000000-0000-4000-8000-000000000002";
const reference = "EX-20260921-11111111-1111-4111-8111-111111111111";
const rule = {
  enabled: true,
  effective_at: "",
  ship_type_id: "12005",
  fitting_id: "1",
  skill_plan_id: "1",
  reference_minor: 0,
  day_zone: "",
  note: "",
  project_name: "伊什塔成长福利",
};
const item = {
  id: "7",
  version: "2",
  account_id: member,
  corporation_id: "10",
  kind: "growth_fitting_1",
  state: "submitted",
  award_minor: 0,
  reference: "WF-7",
  claim_keys: [],
  created_at: "2026-09-21T01:00:00Z",
  updated_at: "2026-09-21T01:00:00Z",
  detail: {
    character_id: "123",
    character_name: "Recipient",
    ship_type_id: "12005",
    description: "",
    evidence: "",
    rule,
    base_minor: 0,
    receipt: "",
    reviewer: "",
    executor: "",
    review_note: "",
    rewards: {
      isk_minor: 50000000,
      fittings: [],
      items: [
        { type_id: "34", quantity: 100, name: "三钛合金" },
        { type_id: "44992", quantity: 2, name: "PLEX" },
      ],
      coins_minor: 100,
    },
  },
};
const lossItem = {
  ...item,
  id: "9",
  kind: "solo",
  detail: {
    ...item.detail,
    character_name: "Loss Pilot",
    ship_type_id: "12005",
    killmail_id: "137007123",
    description: "主动 PVP 损失",
    base_minor: 5352666651,
    rule: { ...rule, project_name: "", loss_rate_bps: 8000, loss_cap_minor: 0 },
    valuation: {
      source: "market",
      state: "ready",
      reason: "",
      at: "2026-09-21T01:01:00Z",
      amount_minor: 5352666651,
      settings_version: "1",
    },
    loss_evidence: {
      id: "137007123",
      character_id: "123",
      corporation_id: "10",
      ship_type_id: "12005",
      ship_name: "厄里斯级",
      solar_system_id: "30000142",
      solar_system_name: "J1-KJP",
      occurred_at: "2026-09-21T00:23:00Z",
      observed_at: "2026-09-21T00:30:00Z",
      damage_taken: 66387,
      attackers: [
        {
          character_id: "9001",
          corporation_id: "1001",
          ship_type_id: "587",
          weapon_type_id: "34",
          name: "Attacker One",
          corporation_name: "Test Corporation",
          alliance_name: "Test Alliance",
          ship_name: "裂谷级",
          weapon_name: "三钛合金",
          damage_done: 52739,
          final_blow: true,
        },
        {
          character_id: "9002",
          corporation_id: "1002",
          ship_type_id: "587",
          name: "Attacker Two",
          ship_name: "裂谷级",
          damage_done: 13648,
          final_blow: false,
        },
      ],
      items: [
        { type_id: "34", name: "三钛合金", slot: "cargo", quantity: 3, destroyed: 3, dropped: 0 },
      ],
    },
  },
};
const lossRow = {
  source: "welfare",
  id: "9",
  version: "2",
  account_id: member,
  applicant: "Main Character",
  corporation_id: "10",
  kind: "solo",
  state: "submitted",
  status: "",
  recipient: "Loss Pilot",
  title: "厄里斯级",
  reference: "WF-9",
  amount_minor: 0,
  unit: "isk",
  time: item.created_at,
  action: "",
  actions: ["approve", "reject", "information"],
  payload: lossItem,
};
const order = {
  id: "8",
  version: "2",
  type_id: "34",
  name: "新兵礼包",
  quantity: 100,
  recipient_id: "123",
  recipient_name: "Recipient",
  coins_minor: 12300,
  isk_per_coin: 10000,
  isk_value: 1230000,
  content: {
    isk_minor: 150000,
    fittings: [],
    items: [{ type_id: "34", quantity: 10, name: "三钛合金" }],
  },
  state: "cancel_requested",
  reference,
  note: "误选奖励",
  created_at: "2026-09-21T02:00:00Z",
  delivery: { status: "waiting_contract", contracts: [], checked_at: null },
};
const rows = [
  {
    source: "welfare",
    id: "7",
    version: "2",
    account_id: member,
    applicant: "Main Character",
    corporation_id: "10",
    kind: item.kind,
    state: "submitted",
    status: "",
    recipient: "Recipient",
    title: rule.project_name,
    reference: "WF-7",
    amount_minor: 0,
    unit: "reward",
    time: item.created_at,
    action: "",
    actions: ["approve", "reject", "information"],
    payload: item,
  },
  {
    source: "exchange",
    id: "8",
    version: "2",
    account_id: member,
    applicant: "Main Character",
    corporation_id: "",
    kind: "exchange",
    state: "cancel_requested",
    status: "waiting_contract",
    recipient: "Recipient",
    title: "新兵礼包",
    reference,
    amount_minor: 12300,
    unit: "coin",
    time: order.created_at,
    action: "",
    actions: ["cancelled", "pending"],
    payload: order,
  },
];
async function setup(
  page: Page,
  { allowed = true, language = "zh-CN", partial = false, loss = false, manyAttackers = false } = {},
) {
  const writes: any[] = [];
  const reads: string[] = [];
  await page.addInitScript(
    (l) => localStorage.setItem("glorynavy.locale", l),
    language,
  );
  await page.route("https://images.evetech.net/**", (r) => r.abort());
  await page.route("**/api/v1/**", (r) => {
    const u = new URL(r.request().url()),
      p = u.pathname;
    reads.push(p + u.search);
    let data: any = {};
    if (p === "/api/v1/modules")
      data = [
        "system",
        "identity",
        "eve",
        "access",
        "exchange",
        "welfare",
        "approval",
      ].map((id) => ({ id, api_version: 1, version: "0.1.0" }));
    else if (p === "/api/v1/identity/session")
      data = {
        authenticated: true,
        session: {
          user_id: allowed ? admin : member,
          character: { id: "123", name: "Reviewer" },
          csrf_token: "csrf",
          expires_at: "2099-01-01T00:00:00Z",
        },
      };
    else if (p === "/api/v1/access/me")
      data = {
        administrator: allowed,
        can_manage: allowed,
        can_manage_sync: false,
        site_roles: [],
        characters: [],
      };
    else if (p === "/api/v1/approval/context")
      data = {
        allowed,
        sources: allowed ? ["welfare", "exchange"] : [],
        corporations: allowed ? [{ id: "10", name: "Glory Navy" }] : [],
        people: allowed ? [{ id: member, name: "Main Character" }] : [],
        unavailable: [],
      };
    else if (p === "/api/v1/approval/items")
      data = {
        items: u.searchParams.get("cursor") ? [] : loss ? [lossRow] : rows,
        counts: { pending: 2, fulfillment: 8, exceptions: 1, history: 12 },
        next_cursor: u.searchParams.get("cursor") ? "" : "page-2",
        unavailable: partial ? ["welfare"] : [],
      };
    else if (p.startsWith("/api/v1/approval/items/"))
      data = structuredClone(
        (loss ? [lossRow] : rows).find((v) => p.endsWith(`/${v.source}/${v.id}`)),
      );
    else if (p === "/api/v1/welfare/context")
      data = {
        administrator: allowed,
        corporations: [{ id: "10", name: "Glory Navy", can_manage: allowed }],
        characters: [
          {
            id: "123",
            name: "Recipient",
            account_id: allowed ? admin : member,
            corporation_id: "10",
          },
        ],
        policies: [
          { corporation_id: "10", kind: loss ? "solo" : item.kind, version: "1", config: loss ? { ...rule, project_name: "", loss_rate_bps: 8000 } : rule },
        ],
      };
    else if (p === "/api/v1/welfare/cases/7")
      data = {
        item,
        history: [{ action: "apply", note: "", created_at: item.created_at }],
      };
    else if (p === "/api/v1/welfare/cases/9")
      data = {
        item: lossItem,
        history: [{ action: "apply", note: "", created_at: item.created_at }],
      };
    else if (p === "/api/v1/welfare/cases")
      data = { items: [], next_cursor: "" };
    else if (
      p === "/api/v1/welfare/commands" ||
      p === "/api/v1/exchange/rewards/orders/8"
    ) {
      expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
      writes.push(r.request().postDataJSON());
      data = { saved: true };
    } else if (p === "/api/v1/exchange/context")
      data = {
        sources: [],
        characters: [{ id: "123", name: "Recipient" }],
        admin: allowed,
      };
    else if (p === "/api/v1/exchange/rewards")
      data = {
        admin: allowed,
        isk_per_coin: 10000,
        version: "1",
        earned_minor: 12300,
        reserved_minor: 12300,
        spent_minor: 0,
        available_minor: 0,
        rewards: [],
        next_cursor: "",
      };
    else if (p === "/api/v1/exchange/rewards/orders")
      data = {
        items: [{ ...order, can_request_cancel: false }],
        next_cursor: "",
      };
    else if (p.endsWith("/handoff"))
      data = {
        reference,
        recipient_id: "123",
        recipient_name: "Recipient",
        state: "pending",
      };
    if (manyAttackers && p === "/api/v1/welfare/cases/9") {
      data = structuredClone(data);
      data.item.detail.loss_evidence.attackers = Array.from({ length: 51 }, (_, index) => ({
        character_id: String(9000 + index),
        name: `Attacker ${index + 1}`,
        damage_done: 51 - index,
        final_blow: index === 50,
      }));
    }
    return r.fulfill({ json: { data } });
  });
  return { writes, reads };
}
test("审批列表、双语和响应式", async ({ page }, info) => {
  await setup(page);
  await page.goto("/approvals");
  await expect(page.getByRole("heading", { name: "审批中心" })).toBeVisible();
  await expect(
    page.getByRole("button", { name: "查看 成长福利 #7" }),
  ).toBeVisible();
  const welfareRow = page.getByRole("row").filter({ hasText: "#7" });
  await expect(welfareRow).toContainText("500,000 ISK");
  await expect(welfareRow).toContainText("三钛合金 ×100");
  await expect(welfareRow).toContainText("PLEX ×2");
  const exchangeRow = page.getByRole("row").filter({ hasText: "#8" });
  await expect(exchangeRow).toContainText("1,500 ISK");
  await expect(exchangeRow).toContainText("123 果壳币");
  await expect(exchangeRow).toContainText("三钛合金 ×10");
  mkdirSync("../docs/ui/reviews/approval", { recursive: true });
  for (const width of [1440, 375, 320]) {
    await page.setViewportSize({ width, height: 900 });
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: `../docs/ui/reviews/approval/${info.project.name}-${width}.png`,
      fullPage: true,
    });
  }
  await page.goto("/approvals?lang=en");
  await expect(
    page.getByRole("heading", { name: "Approval center" }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
});
test("审批页预读其他视图，逐个切换复用已加载数据", async ({ page }) => {
  const { reads } = await setup(page);
  const otherViews = ["fulfillment", "exceptions", "history"];
  const prefetched = otherViews.map((view) =>
    page.waitForResponse((response) => {
      const url = new URL(response.url());
      return url.pathname === "/api/v1/approval/items" && url.searchParams.get("view") === view;
    }),
  );
  await page.goto("/approvals");
  await Promise.all(prefetched);
  await expect(page.getByRole("button", { name: "查看 成长福利 #7" })).toBeVisible();
  for (const [index, view] of otherViews.entries()) {
    const before = reads.filter((path) => path === `/api/v1/approval/items?view=${view}`).length;
    expect(before).toBe(1);
    await page.locator(".approval-tabs button").nth(index + 1).click();
    await expect(page.getByRole("button", { name: "查看 成长福利 #7" })).toBeVisible();
    expect(reads.filter((path) => path === `/api/v1/approval/items?view=${view}`)).toHaveLength(before);
  }
});
test("审批列表预读下一页并复用相同筛选游标", async ({ page }) => {
  const { reads } = await setup(page);
  const nextPage = page.waitForResponse((response) => {
    const url = new URL(response.url());
    return url.pathname === "/api/v1/approval/items" &&
      url.searchParams.get("cursor") === "page-2" &&
      url.searchParams.get("view") === "pending";
  });
  await page.goto("/approvals");
  await nextPage;
  await expect(page.getByRole("button", { name: "下一页" })).toBeEnabled();
  const before = reads.filter((path) => path.includes("cursor=page-2")).length;
  expect(before).toBe(1);
  await page.getByRole("button", { name: "下一页" }).click();
  await expect(page.getByText("暂无符合条件的记录")).toBeVisible();
  expect(reads.filter((path) => path.includes("cursor=page-2"))).toHaveLength(before);
});
test("中心复用审批表单并保留版本和CSRF", async ({ page }) => {
  const { writes } = await setup(page);
  await page.goto("/approvals?source=welfare&id=7");
  const dialog = page.getByRole("dialog");
  await expect(
    dialog.getByRole("button", { name: "批准", exact: true }),
  ).toBeVisible();
  await dialog.getByRole("button", { name: "批准", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(1);
  await dialog.getByRole("textbox", { name: "处理说明" }).fill("核验通过");
  await dialog.getByRole("button", { name: "确认", exact: true }).click();
  await expect.poll(() => writes.length).toBe(1);
  expect(writes[0]).toMatchObject({ action: "approve", id: "7", version: "2" });
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(new URL(page.url()).searchParams.has("source")).toBe(false);
});
test("补损审批先显示损失与金额，证据按需展开", async ({ page }, info) => {
  await setup(page, { loss: true });
  await page.goto("/approvals");
  await expect(page.getByRole("row").filter({ hasText: "#9" })).toContainText("核价金额 · 53,526,666.51 ISK");
  await page.goto("/approvals?source=welfare&id=9");
  const dialog = page.getByRole("dialog");
  const overview = dialog.getByRole("region", { name: "补损审查要点" });
  await expect(overview).toContainText("厄里斯级");
  await expect(overview).toContainText("J1-KJP");
  await expect(overview).toContainText("53,526,666.51 ISK");
  const support = dialog.locator(".welfare-review-support");
  await expect(support).not.toHaveAttribute("open", "");
  await dialog.getByRole("button", { name: /查看KM详情/ }).click();
  await expect(support).toHaveAttribute("open", "");
  await expect(support).toContainText("三钛合金");
  await expect(support).toContainText("参与者 (2)");
  await expect(support).toContainText("承受伤害 66,387");
  await expect(support).toContainText("Attacker One");
  await expect(support).toContainText("Test Corporation · Test Alliance");
  await expect(support).toContainText("最后一击");
  await support.locator("summary").first().click();
  await expect(support).not.toHaveAttribute("open", "");
  await dialog.getByRole("button", { name: "批准", exact: true }).click();
  await expect(dialog.getByText("核准金额")).toBeVisible();
  await expect(dialog.locator(".welfare-loss-preview")).toContainText("42,821,333.2 ISK");
  const previewBox = await dialog.locator(".welfare-loss-preview").boundingBox();
  const supportBox = await support.boundingBox();
  expect(previewBox && supportBox && previewBox.y < supportBox.y).toBe(true);
  mkdirSync("../docs/ui/reviews/approval", { recursive: true });
  await page.screenshot({
    path: `../docs/ui/reviews/approval/loss-review-${info.project.name}.png`,
    fullPage: true,
  });
  await dialog.getByRole("button", { name: /查看KM详情/ }).click();
  await expect(support).toHaveAttribute("open", "");
  await expect(support).toContainText("三钛合金");
  await expect(support).not.toContainText("厄里斯级");
  await expect(support).not.toContainText("53,526,666.51 ISK");
  await page.screenshot({
    path: `../docs/ui/reviews/approval/loss-km-${info.project.name}.png`,
    fullPage: true,
  });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});
test("大量 KM 参与者分批展开并保留最后一击", async ({ page }) => {
  await setup(page, { loss: true, manyAttackers: true });
  await page.goto("/approvals?source=welfare&id=9");
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("button", { name: /查看KM详情/ }).click();
  const support = dialog.locator(".welfare-review-support");
  await expect(support).toContainText("参与者 (51)");
  await expect(support.locator(".welfare-km-attacker")).toHaveCount(5);
  await expect(support).toContainText("Attacker 51");
  await expect(support.getByText("Attacker 25", { exact: true })).toHaveCount(0);
  await support.getByRole("button", { name: /显示更多/ }).click();
  await expect(support.locator(".welfare-km-attacker")).toHaveCount(25);
  await support.getByRole("button", { name: /显示更多/ }).click();
  await expect(support.locator(".welfare-km-attacker")).toHaveCount(45);
  await support.getByRole("button", { name: /显示更多/ }).click();
  await expect(support.locator(".welfare-km-attacker")).toHaveCount(51);
  await support.getByRole("button", { name: "收起" }).click();
  await expect(support.locator(".welfare-km-attacker")).toHaveCount(5);
});
test("兑换取消审核需要理由和未交付确认", async ({ page }) => {
  const { writes } = await setup(page);
  await page.goto("/approvals?source=exchange&id=8");
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("button", { name: "同意取消", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(1);
  const submit = dialog.getByRole("button", { name: /确认取消/ });
  await expect(submit).toBeDisabled();
  await dialog.getByRole("textbox").fill("游戏中核对未发放");
  await dialog.getByRole("checkbox").check();
  await submit.click();
  await expect.poll(() => writes.length).toBe(1);
  expect(writes[0]).toMatchObject({
    state: "cancelled",
    version: "2",
    undelivered_confirmed: true,
  });
});
test("普通成员没有入口，不查询管理列表", async ({ page }) => {
  const { reads } = await setup(page, { allowed: false });
  await page.goto("/approvals");
  await expect(page.getByRole("alert")).toContainText("无权访问审批中心");
  await expect(
    page.getByRole("link", { name: "审批中心", exact: true }),
  ).toHaveCount(0);
  expect(reads.some((p) => p.startsWith("/api/v1/approval/items"))).toBe(false);
});
test("来源失败不显示零计数也不越过失败来源翻页", async ({ page }) => {
  await setup(page, { partial: true });
  await page.goto("/approvals");
  await expect(page.getByRole("alert")).toContainText("部分来源不可用");
  await expect(page.getByRole("button", { name: "下一页" })).toBeDisabled();
  await expect(
    page.getByRole("navigation", { name: "审批视图" }),
  ).toContainText("—");
});
test("原福利页收敛管理记录，保留配置", async ({ page }) => {
  const { reads } = await setup(page);
  await page.goto("/welfare");
  await expect(
    page.getByRole("heading", { name: "军团福利", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("combobox", { name: "申请范围" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "补损设置" })).toBeVisible();
  expect(
    reads
      .filter((p) => p.startsWith("/api/v1/welfare/cases?"))
      .every((p) => !p.includes("all=true")),
  ).toBe(true);
});
test("分页和详情关闭保留筛选", async ({ page }) => {
  await setup(page);
  await page.goto("/approvals?kind=growth");
  await page.getByRole("button", { name: "查看 成长福利 #7" }).click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "关闭", exact: true })
    .click();
  expect(new URL(page.url()).searchParams.get("kind")).toBe("growth");
  await page.getByRole("button", { name: "下一页" }).click();
  await expect(page.getByText("暂无符合条件的记录")).toBeVisible();
  expect(new URL(page.url()).searchParams.get("cursor")).toBe("page-2");
});

test("有福利管理权限的非站点管理员可见入口", async ({ page }) => {
  await setup(page);
  await page.route("**/api/v1/access/me", (r) =>
    r.fulfill({
      json: {
        data: {
          administrator: false,
          can_manage: false,
          can_manage_sync: false,
          site_roles: [],
          characters: [],
        },
      },
    }),
  );
  await page.goto("/approvals");
  await expect(
    page.getByRole("heading", { name: "审批中心", exact: true }),
  ).toBeVisible();
  // Mobile navigation is collapsed, but the same authorized link is present.
  await expect(
    page.getByRole("link", {
      name: "审批中心",
      exact: true,
      includeHidden: true,
    }),
  ).toHaveCount(1);
});

test("原兑换页面只有个人记录与中心链接", async ({ page }) => {
  const { reads } = await setup(page);
  await page.goto("/exchange");
  await expect(
    page.getByRole("heading", { name: "兑换记录", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("combobox", { name: "兑换记录范围" }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "同意取消", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "合同信息", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("link", { name: "前往审批中心", exact: true }),
  ).toBeVisible();
  expect(
    reads
      .filter((p) => p.startsWith("/api/v1/exchange/rewards/orders?"))
      .every((p) => !p.includes("all=true")),
  ).toBe(true);
});
