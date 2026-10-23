import { test, expect, type Page } from "@playwright/test";
const account = "00000000-0000-4000-8000-000000000001";

for(const language of ["zh-CN","en"]){
 test(`feedback fixed reward configuration and acceptance ${language}`,async({page},info)=>{
  await setup(page,language,true);
  const en=language==="en";
  const ticket={...seed,account_id:"00000000-0000-4000-8000-000000000002",accepted_at:undefined as string|undefined,reward_minor:undefined as number|undefined};
  let cfg={amount_minor:0,version:1};
  await page.route("**/api/v1/feedback/reward-settings",async r=>{
   if(r.request().method()==="POST"){const v=r.request().postDataJSON();expect(v.amount_minor).toBe(12345);expect(r.request().headers()["x-csrf-token"]).toBe("csrf");cfg={amount_minor:v.amount_minor,version:2};}
   await r.fulfill({json:{data:cfg}});
  });
  await page.route("**/api/v1/feedback/tickets/1",r=>r.fulfill({json:{data:{ticket,messages:[],administrator:true}}}));
  await page.route("**/api/v1/feedback/tickets/1/messages",async r=>{const v=r.request().postDataJSON();expect(v.status).toBe("accepted");expect(v).not.toHaveProperty("amount_minor");Object.assign(ticket,{status:"accepted",revision:2,accepted_at:"2026-10-23T00:00:00Z",reward_minor:12345});await r.fulfill({json:{data:ticket}});});
  await page.goto("/feedback");
  await page.getByRole("button",{name:en?"Reward settings":"奖励设置",exact:true}).click();
  const dialog=page.getByRole("dialog");const amount=dialog.getByLabel(en?"Adoption reward / Goose Coin":"采纳奖励 / 果壳币");
  await amount.fill("123.456");await expect(dialog.getByRole("button",{name:en?"Save":"保存",exact:true})).toBeDisabled();
  await amount.fill("123.45");await page.screenshot({path:info.outputPath(`settings-${language}.png`),fullPage:true});
  await dialog.getByRole("button",{name:en?"Save":"保存",exact:true}).click();await expect(dialog).toHaveCount(0);
  await page.getByRole("button",{name:/合同页面无法提交申请/}).click();
  await dialog.getByRole("button",{name:en?"Accept feedback":"采纳反馈",exact:true}).click();
  await dialog.getByRole("button",{name:en?"Confirm action":"确认操作",exact:true}).click();
  await expect(dialog.getByText(/123\.45/)).toBeVisible();
  await page.screenshot({path:info.outputPath(`adopted-${language}.png`),fullPage:true});
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
 });
}
const seed = {
  id: "1",
  account_id: account,
  author_name: "Pilot",
  kind: "bug",
  title: "合同页面无法提交申请",
  body: "点击提交后出现错误。\n<script>alert(1)</script>",
  status: "submitted",
  revision: 1,
  created_at: "2026-10-18T00:00:00Z",
  updated_at: "2026-10-18T00:00:00Z",
};
async function setup(
  page: Page,
  language: string,
  admin = false,
  conflict = false,
) {
  const tickets = [{ ...seed }],
    messages: Record<string, unknown>[] = [];
  let attempts = 0;
  await page.addInitScript(
    (l) => localStorage.setItem("glorynavy.locale", l),
    language,
  );
  await page.route("https://images.evetech.net/**", (r) => r.abort());
  await page.route("**/api/v1/**", async (r) => {
    const u = new URL(r.request().url()),
      p = u.pathname,
      post = r.request().method() === "POST";
    let data: unknown = {};
    if (p === "/api/v1/modules")
      data = ["system", "identity", "eve", "access", "feedback"].map((id) => ({
        id,
        api_version: 1,
        version: "0.1.0",
      }));
    if (p === "/api/v1/identity/session")
      data = {
        authenticated: true,
        session: {
          user_id: admin ? "00000000-0000-4000-8000-000000000003" : account,
          character: { id: "123", name: "Pilot" },
          csrf_token: "csrf",
          expires_at: "2099-01-01T00:00:00Z",
        },
      };
    if (p === "/api/v1/access/me")
      data = {
        administrator: admin,
        can_manage: admin,
        can_manage_sync: admin,
        site_roles: [],
        characters: [],
      };
    if (p === "/api/v1/feedback/tickets") {
      if (post) {
        const body = r.request().postDataJSON();
        expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
        expect(body.request_key).toMatch(/^[0-9a-f-]{36}$/);
        const t = { ...seed, ...body, id: "2" };
        tickets.unshift(t);
        data = t;
      } else
        data = {
          tickets: tickets.filter(
            (t) =>
              (!u.searchParams.get("kind") ||
                u.searchParams.get("kind") === t.kind) &&
              (!u.searchParams.get("status") ||
                u.searchParams.get("status") === t.status),
          ),
          administrator: admin,
          next_before: "",
        };
    }
    const match = p.match(/^\/api\/v1\/feedback\/tickets\/(\d+)(\/messages)?$/);
    if (match) {
      const t = tickets.find((t) => t.id === match[1])!;
      if (post) {
        const body = r.request().postDataJSON();
        expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
        if (conflict && attempts++ === 0) {
          t.revision++;
          return r.fulfill({
            status: 409,
            json: {
              error: {
                message:
                  language === "en"
                    ? "This feedback has changed. Refresh and try again."
                    : "反馈已变化，请刷新后重试",
              },
            },
          });
        }
        expect(body.revision).toBe(t.revision);
        t.revision++;
        t.status = body.status || t.status;
        messages.push({
          id: String(messages.length + 1),
          body: body.body,
          status: body.status,
          author_name: admin ? "Admin" : "Pilot",
          staff: admin,
          created_at: seed.created_at,
        });
        data = t;
      } else data = { ticket: t, messages, administrator: admin };
    }
    return r.fulfill({ json: { data } });
  });
}
for (const language of ["zh-CN", "en"]) {
  const t = (zh: string, en: string) => (language === "en" ? en : zh);
  test(`feedback member submit and follow up ${language}`, async ({
    page,
  }, info) => {
    await setup(page, language);
    await page.goto("/feedback");
    await expect(
      page.getByRole("combobox", { name: t("反馈范围", "Feedback scope") }),
    ).toHaveCount(0);
    await page
      .getByRole("button", {
        name: t("提交反馈", "Submit feedback"),
        exact: true,
      })
      .click();
    const dialog = page.getByRole("dialog");
    await dialog.getByRole("combobox").click();
    await page
      .getByRole("option", { name: t("功能需求", "Feature request") })
      .click();
    await dialog
      .getByLabel(t("标题", "Title"), { exact: true })
      .fill("增加合同筛选");
    await dialog
      .getByLabel(t("详细描述", "Description"))
      .fill("按领取状态筛选合同。");
    await dialog
      .getByRole("button", { name: t("提交", "Submit"), exact: true })
      .click();
    await expect(
      page.getByRole("heading", { name: "增加合同筛选", exact: true }),
    ).toBeVisible();
    await expect(dialog.getByRole("combobox")).toHaveCount(0);
    await dialog
      .getByLabel(t("补充说明", "Additional details"))
      .fill("希望保留日期筛选。\n第二行");
    await dialog
      .getByRole("button", { name: t("发送回复", "Send reply") })
      .click();
    await expect(dialog.getByText("希望保留日期筛选。 第二行")).toBeVisible();
    await page.screenshot({
      path: info.outputPath(`member-${language}.png`),
      fullPage: true,
    });
    await expect(page.locator("body")).toHaveJSProperty(
      "scrollWidth",
      await page.evaluate(() => innerWidth),
    );
  });
  test(`feedback administrator handles request ${language}`, async ({
    page,
  }, info) => {
    await setup(page, language, true);
    await page.goto("/feedback");
    await page
      .getByRole("combobox", { name: t("反馈范围", "Feedback scope") })
      .click();
    await page
      .getByRole("option", { name: t("全部反馈", "All feedback") })
      .click();
    await page.screenshot({
      path: info.outputPath(`list-${language}.png`),
      fullPage: true,
    });
    await page.getByRole("button", { name: /合同页面无法提交申请/ }).click();
    const dialog = page.getByRole("dialog");
    await expect(
      dialog.getByText(/<script>alert\(1\)<\/script>/),
    ).toBeVisible();
    for (const label of [t("采纳反馈","Accept feedback"),t("开始处理","Start handling"),t("提交完成","Submit completion")]) {
      await dialog.getByRole("button",{name:label,exact:true}).click();
      if(label===t("提交完成","Submit completion"))await dialog.getByLabel(t("处理结果或原因","Result or reason")).fill("已修复，请刷新后重试。");
      await dialog.getByRole("button",{name:t("确认操作","Confirm action"),exact:true}).click();
    }
    await expect(
      dialog.getByText("已修复，请刷新后重试。", { exact: true }),
    ).toBeVisible();
    await expect(
      dialog.getByText(t("状态变更为 待确认", "Status changed to Pending confirmation")),
    ).toBeVisible();
    await page.screenshot({
      path: info.outputPath(`admin-${language}.png`),
      fullPage: true,
    });
    await expect(page.locator("body")).toHaveJSProperty(
      "scrollWidth",
      await page.evaluate(() => innerWidth),
    );
  });
  test(`feedback stale reply preserves draft ${language}`, async ({ page }) => {
    await setup(page, language, false, true);
    await page.goto("/feedback?ticket=1");
    const dialog = page.getByRole("dialog");
    await dialog
      .getByLabel(t("补充说明", "Additional details"))
      .fill("不能丢失的补充内容");
    await dialog
      .getByRole("button", { name: t("发送回复", "Send reply") })
      .click();
    await expect(dialog.getByRole("alert")).toBeVisible();
    await expect(
      dialog.getByLabel(t("补充说明", "Additional details")),
    ).toHaveValue("不能丢失的补充内容");
    await dialog
      .getByRole("button", { name: t("刷新", "Refresh"), exact: true })
      .click();
    await expect(dialog.getByRole("alert")).toHaveCount(0);
    await dialog
      .getByRole("button", { name: t("发送回复", "Send reply") })
      .click();
    await expect(
      dialog.getByText("不能丢失的补充内容", { exact: true }),
    ).toBeVisible();
  });
}
