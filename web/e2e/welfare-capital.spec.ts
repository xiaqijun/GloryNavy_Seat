import { test, expect, type Page } from "@playwright/test";

const admin = "00000000-0000-4000-8000-000000000001",
  member = "00000000-0000-4000-8000-000000000002";
const rule = {
  enabled: true,
  effective_at: "",
  ship_type_id: "0",
  fitting_id: "0",
  skill_plan_id: "0",
  reference_minor: 0,
  day_zone: "",
  note: "",
  subsidy_rate_bps: 1250,
};
const purchase = {
  id: "901",
  owner_kind: "character",
  owner_id: "123",
  type: "item_exchange",
  status: "finished",
  checked_at: "2026-10-19T00:00:00Z",
  title: "Purchase",
  issuer_id: "456",
  issuer_corporation_id: "10",
  assignee_id: "123",
  acceptor_id: "123",
  price: "10000000000",
  reward: "0",
  issued: "2026-10-18T00:00:00Z",
  completed: "2026-10-18T01:00:00Z",
  content_token: "v1",
  for_corporation: false,
  items_ready: true,
  items: [{ type_id: "23919", quantity: "1", name: "Aeon", included: true }],
};
const preview = {
  contract_id: "901",
  character_id: "123",
  character_name: "Capital Pilot",
  ship_type_id: "23919",
  ship_name: "Aeon",
  price_minor: 1000000000000,
  award_minor: 125000000000,
  completed_at: purchase.completed,
};
const baseItem = {
  id: "99",
  version: "1",
  account_id: member,
  corporation_id: "10",
  kind: "supercarrier",
  state: "submitted",
  award_minor: 0,
  reference: "WF-20261019-000099",
  claim_keys: [],
  created_at: "2026-10-19T00:00:00Z",
  updated_at: "2026-10-19T00:00:00Z",
  detail: {
    character_id: "123",
    character_name: "Capital Pilot",
    ship_type_id: "23919",
    contract_id: "901",
    killmail_id: "0",
    event_id: "0",
    occurred_at: "",
    description: "",
    evidence: "",
    rule,
    base_minor: 1000000000000,
    receipt: "",
    reviewer: "",
    executor: "",
    review_note: "",
    policy_version: "1",
    alliance: "",
    discipline: false,
    purchase,
  },
};

async function setup(
  page: Page,
  language: string,
  review = false,
  initialState = "contract_required",
  caseState = "submitted",
) {
  let state = initialState,
    failed = false;
  const commands: Record<string, any>[] = [];
  const item = {
    ...baseItem,
    state: caseState,
    award_minor: caseState === "submitted" ? 0 : 125000000000,
  };
  const row = {
    source: "welfare",
    id: "99",
    version: "1",
    account_id: member,
    applicant: "Capital Pilot",
    corporation_id: "10",
    kind: "supercarrier",
    state: caseState,
    status: "",
    recipient: "Capital Pilot",
    title: "Aeon",
    reference: item.reference,
    amount_minor: item.award_minor,
    unit: "isk",
    time: item.created_at,
    action: "",
    actions: ["approve", "reject", "information"],
    payload: item,
  };
  await page.addInitScript(
    (l) => localStorage.setItem("glorynavy.locale", l),
    language,
  );
  await page.route("https://images.evetech.net/**", (r) => r.abort());
  await page.route("**/api/v1/**", (r) => {
    const url = new URL(r.request().url()),
      path = url.pathname;
    let data: unknown = {};
    if (path === "/api/v1/modules")
      data = [
        "system",
        "identity",
        "eve",
        "access",
        "exchange",
        "welfare",
        ...(review ? ["approval"] : []),
      ].map((id) => ({ id, api_version: 1, version: "0.1.0" }));
    else if (path === "/api/v1/identity/session")
      data = {
        authenticated: true,
        session: {
          user_id: review ? admin : member,
          character: { id: "123", name: "Capital Pilot" },
          csrf_token: "csrf",
          expires_at: "2099-01-01T00:00:00Z",
        },
      };
    else if (path === "/api/v1/access/me")
      data = {
        administrator: review,
        can_manage: review,
        can_manage_sync: false,
        site_roles: [],
        characters: [],
      };
    else if (path === "/api/v1/welfare/context")
      data = {
        administrator: review,
        corporations: [
          {
            id: "10",
            name: "Glory Navy",
            can_manage: review,
            can_compensate: review,
          },
        ],
        characters: [
          {
            id: "123",
            name: "Capital Pilot",
            account_id: review ? admin : member,
            corporation_id: "10",
          },
        ],
        policies: ["supercarrier", "titan"].map((kind) => ({
          corporation_id: "10",
          kind,
          version: "2",
          config: { ...rule, subsidy_rate_bps: 2500 },
        })),
      };
    else if (path === "/api/v1/welfare/capital/check") {
      if (failed)
        return r.fulfill({
          status: 503,
          json: { error: { message: "Contract unavailable" } },
        });
      const id = url.searchParams.get("contract_id");
      const ready = state === "contract_required" && id === "901";
      data = {
        state: ready
          ? "ready"
          : state === "contract_required" && id
            ? "contract_invalid"
            : state,
        policy_version: "2",
        rate_bps: 1250,
        ...(ready ? { purchase: preview } : {}),
        ...(state === "pending" ? { case_id: "99" } : {}),
      };
    } else if (path === "/api/v1/welfare/cases")
      data = {
        items: url.searchParams.get("kind") === "supercarrier" ? [item] : [],
        next_cursor: "",
      };
    else if (path === "/api/v1/welfare/cases/99")
      data = {
        item,
        history: [
          {
            action: "apply",
            created_at: item.created_at,
            note: "Application received",
          },
        ],
      };
    else if (path === "/api/v1/approval/context")
      data = {
        allowed: true,
        sources: ["welfare"],
        corporations: [{ id: "10", name: "Glory Navy" }],
        people: [{ id: member, name: "Capital Pilot" }],
        unavailable: [],
      };
    else if (path === "/api/v1/approval/items")
      data = {
        items: [row],
        counts: { pending: 1, fulfillment: 0, exceptions: 0, history: 0 },
        next_cursor: "",
        unavailable: [],
      };
    else if (path === "/api/v1/approval/items/welfare/99") data = row;
    else if (path === "/api/v1/welfare/commands") {
      commands.push(r.request().postDataJSON());
      data = { ok: true };
    }
    return r.fulfill({ json: { data } });
  });
  await page.goto(review ? "/approvals?source=welfare&id=99" : "/welfare");
  if (!review)
    await page
      .getByRole("button", {
        name: language === "en" ? "Capital subsidies" : "旗舰补贴",
        exact: true,
      })
      .click();
  return {
    commands,
    setState: (v: string) => {
      state = v;
    },
    fail: (v: boolean) => {
      failed = v;
    },
  };
}

for (const language of ["zh-CN", "en"]) {
  const en = language === "en";
  test(`${language} previews purchase and invalidates it on edits`, async ({
    page,
  }, info) => {
    const ctl = await setup(page, language);
    await page
      .getByRole("button", { name: en ? "Apply" : "申请", exact: true })
      .click();
    const submit = page.getByRole("button", {
      name: en ? "Submit application" : "提交申请",
      exact: true,
    });
    await expect(submit).toBeDisabled();
    const input = page.getByLabel(
      en ? "Ship purchase contract ID" : "购舰合同 ID",
      { exact: true },
    );
    await input.fill("901");
    await expect(submit).toBeEnabled();
    await expect(
      page.getByRole("region", {
        name: en ? "Purchase contract check" : "购舰合同核验",
      }),
    ).toContainText("1.25B ISK");
    await page.screenshot({
      path: `../.local/capital-flow/preview-${language}-${info.project.name}.png`,
      fullPage: true,
    });
    await input.fill("902");
    await expect(submit).toBeDisabled();
    await expect(
      page.getByText(
        en
          ? "Check that the completed personal purchase contract contains the correct ship class and has been synced."
          : "请核对已完成的个人购舰合同及舰船类型，并确认合同已同步。",
        { exact: true },
      ),
    ).toBeVisible();
    await input.fill("901");
    await expect(submit).toBeEnabled();
    await submit.click();
    await expect.poll(() => ctl.commands.length).toBe(1);
    expect(ctl.commands[0]).toMatchObject({
      action: "apply",
      kind: "supercarrier",
      detail: { contract_id: "901" },
    });
    expect(Object.keys(ctl.commands[0].detail)).toEqual(["contract_id"]);
  });
  test(`${language} existing application is accessible and uses frozen rate`, async ({
    page,
  }, info) => {
    await setup(page, language, false, "pending");
    await page
      .getByRole("button", { name: en ? "View application" : "查看申请" })
      .click();
    const dialog = page.getByRole("dialog");
    await expect(dialog).toContainText("12.5%");
    await expect(dialog).not.toContainText("25%");
    await expect(dialog).toContainText("1.25B ISK");
    await expect(
      page.getByRole("list", { name: en ? "Subsidy progress" : "补贴进度" }),
    ).toBeVisible();
    expect(
      await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth),
    ).toBe(true);
    await page.screenshot({
      path: `../.local/capital-flow/detail-${language}-${info.project.name}.png`,
      fullPage: true,
    });
    await expect(
      page.getByRole("button", { name: en ? "Approve" : "批准", exact: true }),
    ).toHaveCount(0);
  });
  test(`${language} approval uses the frozen capital details`, async ({
    page,
  }) => {
    const ctl = await setup(page, language, true);
    await page
      .getByRole("button", { name: en ? "Approve" : "批准", exact: true })
      .click();
    await expect(page.getByRole("dialog")).toContainText("1.25B ISK");
    await page
      .getByLabel(en ? "Action note" : "处理说明", { exact: true })
      .fill("Reviewed purchase");
    await page
      .getByRole("button", { name: en ? "Confirm" : "确认", exact: true })
      .click();
    await expect.poll(() => ctl.commands.length).toBe(1);
    expect(ctl.commands[0]).toMatchObject({
      action: "approve",
      id: "99",
      version: "1",
    });
  });
  test(`${language} payout shows exact contract handoff and current stage`, async ({
    page,
  }, info) => {
    await setup(page, language, true, "pending", "approved");
    await expect(
      page.getByLabel(en ? "Payment amount / ISK" : "支付金额 / ISK", {
        exact: true,
      }),
    ).toHaveValue("1250000000");
    await expect(
      page.getByLabel(en ? "Contract settlement ID" : "合同结算 ID", {
        exact: true,
      }),
    ).toHaveValue(baseItem.reference);
    await expect(
      page.locator('.welfare-capital-progress [aria-current="step"]'),
    ).toContainText(en ? "Approved" : "审批通过");
    await expect(
      page.getByRole("button", {
        name: en ? "Copy contract ID" : "复制合同 ID",
        exact: true,
      }),
    ).toHaveCount(0);
    await page.screenshot({
      path: `../.local/capital-flow/payout-${language}-${info.project.name}.png`,
      fullPage: true,
    });
  });
  test(`${language} errors and claimed status block submission`, async ({
    page,
  }) => {
    const ctl = await setup(page, language);
    ctl.fail(true);
    await page
      .getByRole("button", { name: en ? "Apply" : "申请", exact: true })
      .click();
    await page
      .getByLabel(en ? "Ship purchase contract ID" : "购舰合同 ID", {
        exact: true,
      })
      .fill("901");
    const submit = page.getByRole("button", {
      name: en ? "Submit application" : "提交申请",
      exact: true,
    });
    await expect(page.getByRole("dialog")).toContainText(
      "Contract unavailable",
    );
    await expect(submit).toBeDisabled();
    ctl.fail(false);
    ctl.setState("claimed");
    await page
      .getByRole("dialog")
      .getByRole("button", { name: en ? "Retry" : "重试", exact: true })
      .click();
    await expect(page.getByRole("dialog")).toContainText(
      en ? "Claimed" : "已领取",
    );
    await expect(submit).toBeDisabled();
    expect(ctl.commands).toHaveLength(0);
  });
}
