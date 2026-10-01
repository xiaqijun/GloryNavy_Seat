import { stateLayouts } from "./state-layout";
import { captureDialogLayout } from "./dialog-layout";
import { navigate, openNavigation } from "./navigation";
import { test, expect, type Page } from "@playwright/test";
import { englishLayout } from "./language-layout";
import { mkdirSync } from "node:fs";
import { fileURLToPath } from "node:url";
async function setup(page: Page, manage = true) {
  await page.route("https://images.evetech.net/**", (r) => r.abort());
  await page.route("**/api/v1/modules", (r) =>
    r.fulfill({
      json: {
        data: [
          "system",
          "identity",
          "eve",
          "access",
          "attendance",
          "exchange",
        ].map((id) => ({ id, api_version: 1, version: "0.1.0" })),
      },
    }),
  );
  await page.route("**/api/v1/identity/session", (r) =>
    r.fulfill({
      json: {
        data: {
          authenticated: true,
          session: {
            user_id: "user",
            character: { id: "123", name: "Hajimi1" },
            csrf_token: "csrf",
            expires_at: "2099-01-01T00:00:00Z",
          },
        },
      },
    }),
  );
  await page.route("**/api/v1/access/me", (r) =>
    r.fulfill({
      json: {
        data: {
          administrator: manage,
          can_manage: false,
          can_manage_sync: false,
          site_roles: [],
          characters: [],
        },
      },
    }),
  );

  await page.route("**/api/v1/exchange/context", (r) =>
    r.fulfill({
      json: {
        data: {
          characters: [{ id: "123", name: "Hajimi1" }],
          sources: [
            {
              id: "pap",
              name: "PAP 集结分",
              minor_per_unit: 250,
              version: "1",
            },
          ],
        },
      },
    }),
  );
  await page.route("**/api/v1/exchange/wallet**", (r) =>
    r.fulfill({ json: { data: { items: [], next_cursor: "" } } }),
  );
  await page.route("**/api/v1/exchange/sources", (r) => {
    expect(manage).toBe(true);
    expect(r.request().postDataJSON().minor_per_unit).toBe(350);
    expect(r.request().postDataJSON().mode).toBe("automatic");
    return r.fulfill({ json: { data: { saved: true } } });
  });
}
async function rewardSetup(page: Page, admin = true, debt = false, automatic?: boolean) {
  await setup(page, admin);
  await page.route("**/api/v1/exchange/catalog?**", (r) =>
    r.fulfill({
      json: {
        data: {
          items: [
            {
              id: "1",
              version: "1",
              name: "三钛合金",
              archived: false,
              content: {
                fittings: [],
                items: [{ type_id: "34", name: "三钛合金", quantity: 10 }],
              },
            },
          ],
          next_cursor: "",
        },
      },
    }),
  );
  let rate = 100,
    version = 1,
    reserved_minor = debt ? 10500 : 0,
    spent_minor = 0,
    stock = 2;
  const orders: Record<string, unknown>[] = [];
  await page.route("**/api/v1/exchange/rewards**", async (r) => {
    const url = new URL(r.request().url());
    const path = url.pathname;
    if (r.request().method() === "POST") {
      expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
      const c = r.request().postDataJSON();
      expect(c.request_key).toMatch(/^[0-9a-f-]{36}$/);
      if (path.endsWith("/rate")) {
        expect(admin).toBe(true);
        rate = c.isk_per_coin;
        version++;
      } else if (path.endsWith("/items")) {
        expect(admin).toBe(true);
        expect(c.type_id).toBe("34");
        expect(c.quantity).toBe(10);
      } else if (path.endsWith("/claim")) {
        expect(c.recipient_id).toBe("123");
        expect(c.rate_version).toBe(String(version));
        const coins_minor = Math.ceil(40100 / rate);
        reserved_minor += coins_minor;
        stock--;
        orders.push({
          id: "1",
          version: "1",
          type_id: "34",
          name: "三钛合金",
          quantity: 10,
          recipient_id: "123",
          recipient_name: "Hajimi1",
          coins_minor,
          isk_per_coin: rate,
          isk_value: 401,
          state: "pending",
          can_request_cancel: true,
          reference: "EX-20260921-A2C49E70-91F6-4AF1-8B52-3D8AE61C2097",
          delivery:{status:"waiting_contract",checked_at:null,contracts:[]},
          content:{fittings:[],items:[{type_id:"34",name:"三钛合金",quantity:10}]},
          note: "",
          created_at: "2026-09-15T12:00:00Z",
        });
        return r.fulfill({ json: { data: { id: "1" } } });
      } else if (path.endsWith("/orders/1")) {
        expect(c.version).toBe(orders[0].version);
        expect(c.state).not.toBe("fulfilled");
        if(c.state==="cancelled"){
          expect(admin).toBe(true);expect(c.undelivered_confirmed).toBe(true);expect(orders[0].state).toBe("cancel_requested");
          reserved_minor-=Number(orders[0].coins_minor);stock++;
        }
        Object.assign(orders[0],{state:c.state,note:c.note,can_request_cancel:c.state==="pending",version:String(Number(orders[0].version)+1)});

      } else throw new Error(path);
      return r.fulfill({ json: { data: { saved: true } } });
    }
    if (path.endsWith("/handoff")) { expect(admin).toBe(true); return r.fulfill({json:{data:{reference:"EX-20260921-A2C49E70-91F6-4AF1-8B52-3D8AE61C2097",recipient_id:"123",recipient_name:"Hajimi1",state:orders[0]?.state??"pending"}}}); }
    if (path.endsWith("/valuation"))
      return r.fulfill({
        json: {
          data: {
            version: "1",
            isk_value: 401,
            mid: "400.25",
            complete: true,
            missing_types: 0,
            observed_at: "2026-09-20T00:00:00Z",
          },
        },
      });
    if (path.endsWith("/types"))
      return r.fulfill({ json: { data: [{ id: "34", name: "三钛合金" }] } });
    if (path.endsWith("/orders"))
      return r.fulfill({ json: { data: { items: orders, next_cursor: "" } } });
    return r.fulfill({
      json: {
        data: {
          admin,
          isk_per_coin: rate,
          version: String(version),
          earned_minor: 10000,
          reserved_minor,
          spent_minor,
          available_minor: 10000 - reserved_minor - spent_minor,
          next_cursor: "",
          rewards: [
            {
              id: "1",
              version: "1",
              type_id: "34",
              name: "三钛合金",
              quantity: 10,
              isk_value: 401,
              coins_minor: Math.ceil(40100 / rate),
              ...(automatic === undefined ? {} : { pricing: { automatic, status: automatic ? "ready" : "manual", checked_at: null, next_at: "2099-01-01T00:00:00Z" } }),
              stock,
              enabled: true,
            },
          ],
        },
      },
    });
  });
  await page.goto("/exchange");
  await expect(page.getByText("1 果壳币 = 100 ISK")).toBeVisible();
}
async function navigationSetup(page: Page) {
  await rewardSetup(page);
  await page.route("**/api/v1/modules", (r) =>
    r.fulfill({
      json: {
        data: [
          "system",
          "identity",
          "eve",
          "access",
          "fittings",
          "exchange",
        ].map((id) => ({ id, version: "0.1.0", api_version: 1 })),
      },
    }),
  );
  await page.route("**/api/v1/fittings/library/context", (r) =>
    r.fulfill({
      json: {
        data: {
          can_manage: true,
          corporations: [
            { id: "900", name: "Glory Navy", can_create_skills: true },
          ],
          characters: [{ id: "123", name: "Hajimi1", can_save: true }],
        },
      },
    }),
  );
  await page.route("**/api/v1/fittings/library?**", (r) =>
    r.fulfill({ json: { data: [] } }),
  );
  await page.reload();
}

test("舰船配置与兑换中心按现场速度连续切换", async ({ page }) => {
  test.setTimeout(90000);
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await navigationSetup(page);
  for (let i = 0; i < 30; i++) {
    await navigate(page, "舰船配置", "作战与训练");
    await page.waitForTimeout(170);
    await navigate(page, "兑换中心", "福利与兑换");
    await page.waitForTimeout(170);
  }
  await expect(
    page.getByRole("heading", { name: "兑换中心", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "兑换", exact: true }).click();
  await expect(page.getByRole("form", { name: "确认兑换" })).toBeVisible();
  await page.getByRole("button", { name: "取消", exact: true }).click();
  await expect(page.locator("body")).not.toHaveCSS("pointer-events", "none");
  expect(errors).toEqual([]);
});

test("兑换订单不等待奖励列表响应", async ({ page }) => {
  await rewardSetup(page);
  let shopCompleted = false;
  let ordersBeforeShop = false;
  await page.route("**/api/v1/exchange/rewards?**", async (route) => {
    await new Promise((resolve) => setTimeout(resolve, 500));
    await route.fallback();
    shopCompleted = true;
  });
  await page.route("**/api/v1/exchange/rewards/orders?**", async (route) => {
    ordersBeforeShop = !shopCompleted;
    await route.fallback();
  });
  await page.goto("/exchange");
  await expect(page.getByRole("heading", { name: "兑换中心" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "果壳币流水" })).toBeVisible();
  await expect.poll(() => ordersBeforeShop).toBe(true);
});

test("连续鼠标切换不逐次等待 DOM 稳定", async ({ page, isMobile }) => {
  test.skip(
    isMobile,
    "Desktop mouse reproduction; mobile uses a different navigation layout.",
  );
  test.setTimeout(90000);
  const errors: string[] = [];
  const interventions: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  page.on("console", (message) => {
    if (/Intervention|Throttling navigation/i.test(message.text())) {
      interventions.push(message.text());
    }
  });
  await navigationSetup(page);
  // Accordion groups change link positions. Read each expanded group's bounds,
  // then send mouse input without waiting for destination content to settle.
  for (let i = 0; i < 30; i++) {
    for (const [group, name] of [
      ["作战与训练", "舰船配置"],
      ["福利与兑换", "兑换中心"],
    ]) {
      const nav = await openNavigation(page, group);
      const bounds = await nav
        .getByRole("link", { name, exact: true })
        .boundingBox();
      expect(bounds).not.toBeNull();
      await page.mouse.click(
        bounds!.x + bounds!.width / 2,
        bounds!.y + bounds!.height / 2,
      );
      await page.waitForTimeout(100);
    }
  }
  await expect(
    page.getByRole("heading", { name: "兑换中心", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "兑换", exact: true }).click();
  await expect(page.getByRole("form", { name: "确认兑换" })).toBeVisible();
  expect(errors).toEqual([]);
  expect(interventions).toEqual([]);
});

test("English exchange layout", async ({ page }, info) => {
  await rewardSetup(page);
  await englishLayout(page, "Exchange", `exchange-${info.project.name}`);
});

test("rewards member claim and cancel preserve balance", async ({ page }) => {
  await rewardSetup(page, false);
  await expect(
    page.getByRole("button", { name: "果壳币价值", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("combobox", { name: "兑换记录范围" }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "兑换", exact: true }).click();
  await expect(page.getByRole("combobox", { name: "接收角色" })).toBeFocused();
  await expect(page.getByRole("form", { name: "确认兑换" })).toContainText(
    "4.01 币",
  );
  await page.getByRole("button", { name: "确认兑换", exact: true }).click();
  await expect(page.getByText("#1 · 待发放")).toBeVisible();
  await expect(page.getByRole("button", { name: "标记已发放" })).toHaveCount(0);
  await page.getByRole("button", { name: "申请取消", exact: true }).click();
  await page.getByLabel("取消原因").fill("暂不领取");
  await page.getByRole("button", { name: "提交申请", exact: true }).click();
  await expect(page.getByText("#1 · 取消待审核")).toBeVisible();
  await expect(
    page.locator(".exchange-metric").filter({ hasText: "可用果壳币" }),
  ).toContainText("95.99");
  await expect(page.getByRole("button",{name:"同意取消"})).toHaveCount(0);
});
test("rewards admin pricing local SDE and fulfillment", async ({ page }) => {
  await rewardSetup(page);
  for (const name of [
    "果壳币价值",
    "PAP兑换比例",
    "上架奖励",
    "编辑 三钛合金",
  ]) {
    await expect(page.getByRole("button", { name, exact: true })).toHaveCount(
      0,
    );
  }
  await page.goto("/rewards");
  await page.getByRole("button", { name: "果壳币价值", exact: true }).click();
  await page.getByLabel("每 1 果壳币的 ISK 价值").fill("200");
  await page.getByRole("button", { name: "保存", exact: true }).click();
  await expect(page.getByRole("form", { name: "果壳币价值设置" })).toHaveCount(
    0,
  );
  await page.getByRole("button", { name: "兑换设置" }).click();
  await expect(page.getByLabel("可用库存（份）")).toBeFocused();
  await expect(page.getByLabel("价值")).toHaveValue("401");
  await expect(page.getByLabel("价值", { exact: true })).toBeEditable();
  await page.getByRole("button", { name: "保存", exact: true }).click();
  await expect(page.getByRole("dialog", { name: "兑换设置" })).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "兑换设置", exact: true }),
  ).toBeFocused();
  await page.goto("/exchange");
  await expect(page.getByText("1 果壳币 = 200 ISK")).toBeVisible();
  await page.getByRole("button", { name: "兑换", exact: true }).click();
  await page.getByRole("button", { name: "确认兑换", exact: true }).click();
  await expect(page.getByRole("button",{name:"标记已发放"})).toHaveCount(0);
  await expect(page.getByRole("button",{name:"复制合同结算 ID"})).toContainText("EX-20260921-A2C49E70-91F6-4AF1-8B52-3D8AE61C2097");
  await page.getByText("奖励内容",{exact:true}).click();
  await expect(page.locator(".exchange-delivery-details")).toContainText("三钛合金");
  await page.getByRole("button",{name:"申请取消",exact:true}).click();
  await page.getByLabel("取消原因").fill("暂不领取");
  await page.getByRole("button",{name:"提交申请",exact:true}).click();
  await page.getByRole("button",{name:"驳回取消",exact:true}).click();
  await page.getByLabel("驳回原因").fill("已准备合同");
  await page.getByRole("button",{name:"确认驳回",exact:true}).click();
  await expect(page.getByText("#1 · 待发放")).toBeVisible();
  await page.getByRole("button",{name:"申请取消",exact:true}).click();
  await page.getByLabel("取消原因").fill("已确认不领取");
  await page.getByRole("button",{name:"提交申请",exact:true}).click();
  await page.getByRole("button",{name:"同意取消",exact:true}).click();
  await page.getByLabel("核对说明").fill("没有已交付合同");
  await expect(page.getByRole("button",{name:"确认取消",exact:true})).toBeDisabled();
  await page.getByRole("checkbox").check();
  await page.getByRole("button",{name:"确认取消",exact:true}).click();
  await expect(page.getByText("#1 · 已取消")).toBeVisible();
  await expect(page.locator(".exchange-metric").filter({hasText:"可用果壳币"})).toContainText("100");

});
test("rewards debt and responsive layout", async ({ page }, info) => {
  await rewardSetup(page, true, true);
  await expect(page.getByRole("alert")).toContainText("果壳币欠额 5");
  await expect(
    page.getByRole("button", { name: "兑换", exact: true }),
  ).toBeDisabled();
  for (const width of [1440, 375, 320]) {
    await page.setViewportSize({ width, height: 1000 });
    await expect
      .poll(() =>
        page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
      )
      .toBe(true);
    const dir = fileURLToPath(
      new URL("../../docs/ui/reviews/exchange/", import.meta.url),
    );
    mkdirSync(dir, { recursive: true });
    await page.screenshot({
      path: `${dir}/rewards-${info.project.name}-${width}.png`,
      fullPage: true,
    });
  }
});

test("configure automatic PAP coin ratio", async ({ page }) => {
  await rewardSetup(page);
  await page.goto("/rewards");
  await page.getByRole("button", { name: "PAP兑换比例" }).click();
  await page.getByLabel("每 1 PAP 发放果壳币").fill("3.50");
  await page.getByRole("combobox", { name: "兑换方式" }).click();
  await page.getByRole("option", { name: "自动兑换", exact: true }).click();
  await page.getByRole("button", { name: "保存", exact: true }).click();
  await expect(page.getByRole("form", { name: "发币比例设置" })).toHaveCount(0);
});

stateLayouts({
  name: "exchange",
  setup: rewardSetup,
  path: "/exchange",
  endpoint: "**/api/v1/exchange/context",
});

for (const outcome of ["complete", "incomplete", "error"] as const) {
  test(`automatic reward pricing immediately refreshes value: ${outcome}`, async ({ page }) => {
    await rewardSetup(page, true, false, false);
    let calls = 0;
    let refreshing = false;
    let release!: () => void;
    const pending = new Promise<void>((resolve) => { release = resolve; });
    await page.route("**/api/v1/exchange/rewards/1/valuation?**", async (route) => {
      calls++;
      const refresh = refreshing;
      if (refresh) await pending;
      if (refresh && outcome === "error") return route.fulfill({ status: 503, json: { error: { message: "核价失败" } } });
      const complete = !refresh || outcome === "complete";
      return route.fulfill({ json: { data: { version: "1", isk_value: complete ? 975 : 0, mid: complete ? "974.50" : "0.00", complete, missing_types: complete ? 0 : 1, observed_at: null } } });
    });
    let submitted: any;
    await page.route("**/api/v1/exchange/rewards/items", (route) => {
      submitted = route.request().postDataJSON();
      return route.fulfill({ json: { data: { saved: true } } });
    });
    await page.goto("/rewards");
    await page.getByRole("button", { name: "兑换设置", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "兑换设置" });
    const value = dialog.getByRole("spinbutton", { name: "价值", exact: true });
    const mode = dialog.getByRole("combobox", { name: "定价方式" });
    const save = dialog.getByRole("button", { name: "保存", exact: true });
    await expect(value).toBeEnabled();
    await expect(value).toHaveValue("401"); // Opening never overwrites a saved manual price.
    await value.fill("600");
    refreshing = true;
    const previousCalls = calls;
    await mode.click();
    await page.getByRole("option", { name: "自动核价 · 每 6 小时", exact: true }).click();
    await expect.poll(() => calls).toBeGreaterThan(previousCalls);
    await expect(dialog.getByRole("status")).toHaveText("正在核价");
    await expect(value).toHaveValue("600");
    await expect(value).toBeDisabled();
    await expect(mode).toBeDisabled();
    await expect(save).toBeDisabled();
    release();
    await expect(value).toBeEnabled();
    await expect(value).toHaveValue(outcome === "complete" ? "975" : "600");
    if (outcome !== "complete") await expect(dialog.getByRole("alert")).toContainText(outcome === "error" ? "核价失败" : "报价不完整");
    await expect(mode).toContainText("自动核价");
    await save.click();
    await expect(dialog).toHaveCount(0);
    expect(submitted).toMatchObject({ automatic_pricing: true, isk_value: outcome === "complete" ? 975 : 600 });
  });
}

test("reward valuation missing quotes blocks saving until refreshed", async ({
  page,
}) => {
  await rewardSetup(page);
  let complete = false,
    saved = 0;
  await page.route("**/api/v1/exchange/rewards/1/valuation?**", (r) =>
    r.fulfill({
      json: {
        data: {
          version: "1",
          isk_value: complete ? 501 : 0,
          mid: complete ? "500.12" : "0.00",
          complete,
          missing_types: complete ? 0 : 1,
          observed_at: null,
        },
      },
    }),
  );
  await page.route("**/api/v1/exchange/rewards/items", (r) => {
    expect(r.request().postDataJSON().isk_value).toBe(501);
    saved++;
    return r.fulfill({ json: { data: { saved: true } } });
  });
  await page.goto("/rewards");
  await page.getByRole("button", { name: "兑换设置", exact: true }).click();
  const form = page.getByRole("dialog", { name: "兑换设置" });
  await expect(form.getByRole("alert")).toContainText("报价不完整");
  await expect(
    form.getByRole("button", { name: "保存", exact: true }),
  ).toBeDisabled();
  await form.getByLabel("可用库存（份）").fill("7");
  complete = true;
  await form.getByRole("button", { name: "重新核价", exact: true }).click();
  await expect(form.getByLabel("价值")).toHaveValue("501");
  await expect(form.getByLabel("可用库存（份）")).toHaveValue("7");
  for (const width of [1440, 375, 320]) {
    await page.setViewportSize({ width, height: 900 });
    await expect
      .poll(() =>
        page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
      )
      .toBe(true);
  }
  await form.getByRole("button", { name: "保存", exact: true }).click();
  await expect(form).toHaveCount(0);
  expect(saved).toBe(1);
});

for (const complete of [true,false]) {
  test('reward manual value overrides '+(complete?'market quote':'missing quote'), async({page})=>{
    await rewardSetup(page);
    let saved=0;
    await page.route("**/api/v1/exchange/rewards/1/valuation?**",r=>r.fulfill({json:{data:{version:"1",isk_value:complete?401:0,mid:complete?"400.25":"0.00",complete,missing_types:complete?0:1,observed_at:null}}}));
    await page.route("**/api/v1/exchange/rewards/items",r=>{expect(r.request().postDataJSON().isk_value).toBe(600);saved++;return r.fulfill({json:{data:{saved:true}}});});
    await page.goto("/rewards");
    await page.getByRole("button",{name:"兑换设置",exact:true}).click();
    const form=page.getByRole("dialog",{name:"兑换设置"});
    const value=form.getByRole("spinbutton",{name:"价值",exact:true});
    await expect(value).toBeEnabled();
    await value.fill("0");
    await expect(form.getByRole("button",{name:"保存",exact:true})).toBeDisabled();
    await value.fill("600");
    if(!complete){
      await form.getByRole("button",{name:"重新核价",exact:true}).click();
      await expect(value).toBeEnabled();
      await expect(value).toHaveValue("600");
    }
    await form.getByRole("button",{name:"保存",exact:true}).click();
    await expect(form).toHaveCount(0);expect(saved).toBe(1);
  });
}


test("admin contract information copies only recipient and settlement ID",async({page})=>{
 await page.addInitScript(()=>Object.defineProperty(navigator,"clipboard",{configurable:true,value:{writeText:async(text:string)=>{document.documentElement.dataset.copied=text;}}}));
 await rewardSetup(page);
 await page.getByRole("button",{name:"兑换",exact:true}).click();
 await page.getByRole("button",{name:"确认兑换",exact:true}).click();
 await page.getByRole("button",{name:"合同信息",exact:true}).click();
 const dialog=page.getByRole("dialog",{name:"合同信息"});
 await expect(dialog.getByLabel("接收角色",{exact:true})).toHaveValue("Hajimi1");
 await expect(dialog.getByRole("textbox")).toHaveCount(2);
 await expect(dialog.getByLabel("合同结算 ID",{exact:true})).toHaveValue("EX-20260921-A2C49E70-91F6-4AF1-8B52-3D8AE61C2097");
 await expect(dialog.getByRole("button",{name:"复制交付信息",exact:true})).toHaveCount(0);
 for(const [label,value]of [["复制接收角色","Hajimi1"],["复制合同结算 ID","EX-20260921-A2C49E70-91F6-4AF1-8B52-3D8AE61C2097"]]){
  await dialog.getByRole("button",{name:label,exact:true}).click();
  await expect.poll(()=>page.evaluate(()=>document.documentElement.dataset.copied)).toBe(value);
 }
 for(const width of [1440,375,320]){await page.setViewportSize({width,height:900});await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);}
 await dialog.getByRole("button",{name:"关闭",exact:true}).click();
 await expect(page.getByRole("button",{name:"合同信息",exact:true})).toBeFocused();
 await page.getByRole("button",{name:"申请取消",exact:true}).click();await page.getByLabel("取消原因").fill("取消");await page.getByRole("button",{name:"提交申请",exact:true}).click();
 await page.getByRole("button",{name:"合同信息",exact:true}).click();
 await expect(dialog).toContainText("请勿创建交付合同");
});

test("cash contract information copies whole ISK", async ({ page }) => {
  await page.addInitScript(() =>
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: {
        writeText: async (value: string) => {
          document.documentElement.dataset.copied = value;
        },
      },
    }),
  );
  await rewardSetup(page);
  await page.route("**/api/v1/exchange/rewards/orders/1/handoff", (route) =>
    route.fulfill({
      json: {
        data: {
          reference: "EX-20260921-A2C49E70-91F6-4AF1-8B52-3D8AE61C2097",
          recipient_id: "123",
          recipient_name: "Hajimi1",
          amount: "123",
          state: "pending",
        },
      },
    }),
  );
  await page.getByRole("button", { name: "兑换", exact: true }).click();
  await page.getByRole("button", { name: "确认兑换", exact: true }).click();
  await page.getByRole("button", { name: "合同信息", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "合同信息" });
  await expect(dialog.getByLabel("支付金额 / ISK", { exact: true })).toHaveValue(
    "123",
  );
  await dialog.getByRole("button", { name: "复制支付金额" }).click();
  await expect
    .poll(() => page.evaluate(() => document.documentElement.dataset.copied))
    .toBe("123");
});

test("cancellation dialogs retain input, block pending dismissal and restore focus", async ({page}, info) => {
 await rewardSetup(page);
 await page.getByRole('button',{name:'兑换',exact:true}).click();
 await page.getByRole('button',{name:'确认兑换',exact:true}).click();
 const trigger=page.getByRole('button',{name:'申请取消',exact:true});
 await trigger.click();
 const dialog=page.getByRole('dialog',{name:'申请取消',exact:true});
 await expect(dialog.getByLabel('取消原因')).toBeFocused();
 await dialog.getByLabel('取消原因').fill('   ');
 await expect(dialog.getByRole('button',{name:'提交申请'})).toBeDisabled();
 await page.keyboard.press('Escape');
 await expect(dialog).toHaveCount(0);await expect(trigger).toBeFocused();
 await trigger.click();
 await dialog.getByLabel('取消原因').fill('暂不领取，保留输入');
 let attempts=0; const keys:string[]=[];
 let release!:()=>void; const gate=new Promise<void>(resolve=>{release=resolve});
 await page.route('**/api/v1/exchange/rewards/orders/1',async route=>{
   attempts++; keys.push(route.request().postDataJSON().request_key);
   if(attempts===1)return route.fulfill({status:503,json:{error:{message:'暂时无法连接服务，请稍后重试'}}});
   await gate;await route.fallback();
 });
 await dialog.getByRole('button',{name:'提交申请'}).click();
 await expect(dialog.getByRole('alert')).toBeVisible();
 await expect(dialog.getByLabel('取消原因')).toHaveValue('暂不领取，保留输入');
 await dialog.getByRole('button',{name:'提交申请'}).click();
 await expect(dialog.getByRole('button',{name:'正在提交'})).toBeDisabled();
 await expect(dialog.getByRole('button',{name:'关闭',exact:true})).toBeDisabled();
 await page.keyboard.press('Escape');await expect(dialog).toBeVisible();
 release();await expect(dialog).toHaveCount(0);expect(attempts).toBe(2);expect(keys[0]).toBe(keys[1]);
 const dir='../.local/exchange-decision-dialogs/screenshots';mkdirSync(dir,{recursive:true});
 for(const title of ['驳回取消','同意取消']){
   const opener=page.getByRole('button',{name:title,exact:true});await opener.click();
   const modal=page.getByRole('dialog',{name:title,exact:true});
   if(title==='同意取消'){
     await expect(modal.getByRole('button',{name:'取消',exact:true})).toBeFocused();
     await expect(modal.locator('.exchange-refund-summary')).toContainText('4.01');
     await expect(modal.getByRole('button',{name:'确认取消',exact:true})).toBeDisabled();
   }else await expect(modal.getByLabel('驳回原因')).toBeFocused();
   for(const width of [1440,375,320]){
     await page.setViewportSize({width,height:720});
     await expect.poll(()=>modal.evaluate(e=>e.scrollWidth<=e.clientWidth)).toBe(true);
     await expect(modal.getByRole('button',{name:title==='同意取消'?'确认取消':'确认驳回',exact:true})).toBeInViewport();
     await page.screenshot({path:`${dir}/${info.project.name}-${title}-${width}.png`});
   }
   await modal.getByRole('button',{name:'取消',exact:true}).click();await expect(opener).toBeFocused();
 }
 await page.getByRole('button',{name:'切换为英文',exact:true}).click();
 await page.getByRole('button',{name:'Approve cancellation',exact:true}).click();
 await expect(page.getByRole('dialog')).toContainText('Coin refund');
 await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
 await page.screenshot({path:`${dir}/${info.project.name}-approve-en-320.png`});
});

test("claim dialog preserves selection and separates dropdown dismissal", async({page},info)=>{
 await rewardSetup(page,false);
 const opener=page.getByRole('button',{name:'兑换',exact:true});await opener.click();
 const dialog=page.getByRole('dialog',{name:'确认兑换',exact:true});
 const recipient=dialog.getByRole('combobox',{name:'接收角色'});
 await expect(recipient).toBeFocused();await recipient.click();
 await expect(page.getByRole('option',{name:'Hajimi1'})).toBeVisible();
 await page.keyboard.press('Escape');await expect(dialog).toBeVisible();await expect(recipient).toBeFocused();
 await page.keyboard.press('Escape');await expect(dialog).toHaveCount(0);await expect(opener).toBeFocused();
 await opener.click();
 const dir='../.local/exchange-claim-dialog/screenshots';mkdirSync(dir,{recursive:true});
 for(const width of [1440,375,320]){
   await page.setViewportSize({width,height:720});
   await expect.poll(()=>dialog.evaluate(e=>e.scrollWidth<=e.clientWidth)).toBe(true);
   await expect(dialog.getByRole('button',{name:'确认兑换',exact:true})).toBeInViewport();
   await page.screenshot({path:`${dir}/${info.project.name}-${width}.png`});
 }
 let attempts=0; const keys:string[]=[];
 let release!:()=>void; const gate=new Promise<void>(resolve=>{release=resolve});
 await page.route('**/api/v1/exchange/rewards/claim',async route=>{
   attempts++;const body=route.request().postDataJSON(); keys.push(body.request_key);expect(body.recipient_id).toBe('123');
   if(attempts===1)return route.fulfill({status:503,json:{error:{message:'暂时无法连接服务，请稍后重试'}}});
   await gate;await route.fallback();
 });
 await dialog.getByRole('button',{name:'确认兑换',exact:true}).click();
 await expect(dialog.getByRole('alert')).toBeVisible();await expect(recipient).toContainText('Hajimi1');
 await dialog.getByRole('button',{name:'确认兑换',exact:true}).click();
 await expect(dialog.getByRole('button',{name:'正在提交'})).toBeDisabled();
 await expect(dialog.getByRole('button',{name:'关闭',exact:true})).toBeDisabled();
 await expect(recipient).toBeDisabled();await page.keyboard.press('Escape');await expect(dialog).toBeVisible();
 release();await expect(dialog).toHaveCount(0);expect(attempts).toBe(2);expect(keys[0]).toBe(keys[1]);
 await expect(page.getByText('#1 · 待发放')).toBeVisible();
 await page.getByRole('button',{name:'切换为英文',exact:true}).click();
 await page.getByRole('button',{name:'Redeem',exact:true}).click();
 const english=page.getByRole('dialog',{name:'Confirm conversion',exact:true});
 await expect(english).toBeVisible();await expect.poll(()=>english.evaluate(e=>e.scrollWidth<=e.clientWidth)).toBe(true);
 await page.screenshot({path:`${dir}/${info.project.name}-en-320.png`});
});

test("reward settings dialogs keep focus and footer on narrow screens", async({page},info)=>{
 await rewardSetup(page);await page.goto('/rewards');
 for(const [button,title] of [['果壳币价值','果壳币价值设置'],['PAP兑换比例','发币比例设置'],['兑换设置','兑换设置']]){
   const trigger=page.getByRole('button',{name:button,exact:true});await trigger.click();
   await captureDialogLayout(page,title,`${info.project.name}-${button}`);
   const dialog=page.getByRole('dialog',{name:title,exact:true});
   if(button==='PAP兑换比例'){
     await dialog.getByRole('combobox',{name:'兑换方式'}).click();await page.keyboard.press('Escape');await expect(dialog).toBeVisible();
   }
   await page.keyboard.press('Escape');await expect(dialog).toHaveCount(0);await expect(trigger).toBeFocused();
 }
});
