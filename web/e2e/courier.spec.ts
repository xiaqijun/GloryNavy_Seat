import { test, expect, type Page } from "@playwright/test";
const baseRoute = { id: 1, start: "Jita IV - Moon 4 - Caldari Navy Assembly Plant", end: "1DH-SX III - Moon 1 - Blood Raiders Logistic Support", cost_per_m3: 600, collateral_percentage: 1, transport_type: "JF full max", transport_volume: 375000, paused: 0, expiration: 14, completion: 7, minimum_cost: 10000000 };
async function setup(page: Page, language: string, failed = false) {
  await page.addInitScript(l => { localStorage.setItem("glorynavy.locale", l); Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText: async (text: string) => { document.documentElement.dataset.copied = text; } } }); }, language);
  await page.route("https://images.evetech.net/**", r => r.abort());
  await page.route("**/api/v1/**", r => {
    const p = new URL(r.request().url()).pathname;
    let data: unknown = {};
    if (p === "/api/v1/modules") data = ["system", "identity", "eve", "access", "courier"].map(id => ({id,api_version:1,version:"0.1.0"}));
    if (p === "/api/v1/identity/session") data = {authenticated:true,session:{user_id:"00000000-0000-4000-8000-000000000001",character:{id:"123",name:"Pilot"},csrf_token:"csrf",expires_at:"2099-01-01T00:00:00Z"}};
    if (p === "/api/v1/access/me") data = {administrator:false,can_manage:false,can_manage_sync:false,site_roles:[],characters:[]};
    if (p === "/api/v1/courier/routes") data = {routes:[baseRoute,{...baseRoute,id:2,start:"Paused origin",paused:1}],location_categories:{[baseRoute.start]:{category_name:"High-sec",category_priority:0}},fetched_at:"2026-10-18T00:00:00Z"};
    if (p === "/api/v1/courier/appraisal") {
      expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
      expect(r.request().postDataJSON().appraisal_text).toBe("Tritanium 3");
      data = {from_cache:false,appraisal:{buy_price:900000000,sell_price:1000000000,volume:20000,failed_items:failed?{Unknown:1}:{},janice_url:null}};
    }
    return r.fulfill({json:{data}});
  });
}
for (const language of ["zh-CN", "en"]) {
  const en = language === "en";
  const t = (zh: string, english: string) => en ? english : zh;
  async function choose(page: Page) {
    await page.getByRole("combobox",{name:t("起点","Start"),exact:true}).click();
    await expect(page.getByRole("option",{name:"Paused origin",exact:true})).toHaveCount(0);
    await page.getByRole("option",{name:baseRoute.start,exact:true}).click();
  }
  test(`courier manual quote and copy ${language}`, async ({page}, info) => {
    await setup(page,language);await page.goto("/courier");
    await page.getByRole("button",{name:t("手动填写","Manual entry"),exact:true}).click();
    await page.getByLabel(t("货物体积 / m³","Cargo volume / m³"),{exact:true}).fill("20000");
    await page.getByLabel(t("押金 / ISK","Collateral / ISK"),{exact:true}).fill("1000000000");
    await choose(page);
    await expect(page.getByLabel(t("报酬 / ISK","Reward / ISK"),{exact:true})).toHaveValue("22000000");
    await page.getByRole("button",{name:t("复制 报酬 / ISK","Copy Reward / ISK"),exact:true}).click();
    await expect(page.locator("html")).toHaveAttribute("data-copied","22000000");
    await page.screenshot({path:info.outputPath(`manual-${language}.png`),fullPage:true});
    expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
    await page.getByLabel(t("货物体积 / m³","Cargo volume / m³"),{exact:true}).fill("375001");
    await expect(page.getByRole("alert")).toContainText(t("体积超出路线运量","Cargo exceeds"));
    await expect(page.getByRole("button",{name:t("复制合同信息","Copy contract details"),exact:true})).toBeDisabled();
  });
  test(`courier appraisal invalidates after editing ${language}`, async ({page},info) => {
    await setup(page,language);await page.goto("/courier");await choose(page);
    await expect(page.getByRole("button",{name:t("复制合同信息","Copy contract details"),exact:true})).toBeDisabled();
    await page.getByLabel(t("物品清单","Item list"),{exact:true}).fill("Tritanium 3");
    await page.getByRole("button",{name:t("估价","Appraise"),exact:true}).click();
    await expect(page.getByLabel(t("报酬 / ISK","Reward / ISK"),{exact:true})).toHaveValue("22000000");
    await expect(page.getByRole("button",{name:t("复制合同信息","Copy contract details"),exact:true})).toBeEnabled();
    await page.screenshot({path:info.outputPath(`appraisal-${language}.png`),fullPage:true});
    expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
    await page.getByLabel(t("物品清单","Item list"),{exact:true}).fill("Tritanium 4");
    await expect(page.getByRole("button",{name:t("复制合同信息","Copy contract details"),exact:true})).toBeDisabled();
  });
  test(`courier unknown cargo cannot copy ${language}`, async ({page}) => {
    await setup(page,language,true);await page.goto("/courier");await choose(page);
    await page.getByLabel(t("物品清单","Item list"),{exact:true}).fill("Tritanium 3");
    await page.getByRole("button",{name:t("估价","Appraise"),exact:true}).click();
    await expect(page.getByRole("alert")).toContainText("Unknown");
    await expect(page.getByRole("button",{name:t("复制合同信息","Copy contract details"),exact:true})).toBeDisabled();
  });
  test(`courier route error retry ${language}`, async ({page}) => {
    await setup(page,language);await page.route("**/api/v1/courier/routes",r=>r.fulfill({status:503,json:{error:{message:"Unavailable"}}}));
    await page.goto("/courier");await expect(page.getByRole("alert")).toContainText(t("路线读取失败","Unable to load routes"));
    await expect(page.getByRole("combobox",{name:t("起点","Start"),exact:true})).toBeDisabled();
    await page.unroute("**/api/v1/courier/routes");await page.getByRole("button",{name:t("刷新路线","Refresh routes"),exact:true}).click();await choose(page);
  });
}
