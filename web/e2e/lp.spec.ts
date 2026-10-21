import { test, expect, type Page } from "@playwright/test";
const offers = Array.from({length:26},(_,i)=>({offer_id:i+1,type_id:10+i,quantity:2,lp_cost:100+i,ak_cost:0,isk_cost:50,required_items:[{type_id:99,quantity:3}]}));
const names = Object.fromEntries([...offers.map(o=>[o.type_id,{zh:o.type_id===10?"姐妹会核心扫描探针":"兑换物品 "+o.type_id,en:o.type_id===10?"Sisters Core Scanner Probe":"Item "+o.type_id}]),[99,{zh:"三钛合金",en:"Tritanium"}]]);
async function setup(page:Page,language:string) {
  await page.addInitScript(l=>localStorage.setItem("glorynavy.locale",l),language);
  await page.route("https://images.evetech.net/**",r=>r.abort());
  await page.route("**/api/v1/**",r=>{
    const url=new URL(r.request().url());const path=url.pathname;let data:unknown={};
    if(path==="/api/v1/modules")data=["system","identity","eve","access","lp"].map(id=>({id,api_version:1,version:"0.1.0"}));
    if(path==="/api/v1/identity/session")data={authenticated:true,session:{user_id:"00000000-0000-4000-8000-000000000001",character:{id:"123",name:"Pilot"},csrf_token:"csrf",expires_at:"2099-01-01T00:00:00Z"}};
    if(path==="/api/v1/access/me")data={administrator:false,can_manage:false,can_manage_sync:false,site_roles:[],characters:[]};
    if(path==="/api/v1/lp/catalog")data={build:123,corporations:[{id:1000130,zh:"姐妹会",en:"Sisters of EVE",faction_id:500016},{id:1000035,zh:"加达里海军",en:"Caldari Navy",faction_id:500001}],factions:[{id:500016,zh:"姐妹会",en:"Servant Sisters of EVE"},{id:500001,zh:"加达里合众国",en:"Caldari State"}]};
    if(path==="/api/v1/lp/offers")data={corporation_id:Number(url.searchParams.get("corporation")),sde_build:123,offers:url.searchParams.get("corporation")==="1000035"?[]:offers,names,observed_at:"2026-10-21T00:00:00Z"};
    if(path==="/api/v1/lp/prices")data={items:Object.fromEntries(url.searchParams.get("types")!.split(",").map(id=>[id,{buy:id==="99"?5:100,sell:id==="99"?10:200}])),observed_at:"2026-10-21T00:00:00Z"};
    return r.fulfill({json:{data}});
  });
}
for(const language of ["zh-CN","en"]) {
  const t=(zh:string,en:string)=>language==="en"?en:zh;
  test(`LP query search details pricing and layout ${language}`,async({page},info)=>{
    await setup(page,language);await page.goto("/lp");
    await expect(page.getByRole("heading",{name:t("LP 兑换查询","LP exchange lookup")})).toBeVisible();
    await expect(page.getByText("3.2 ISK/LP",{exact:true})).toBeVisible();
    await page.getByLabel(t("搜索兑换物品","Search exchange items")).fill("Sisters Core");
    await expect(page.locator("tbody tr")).toHaveCount(1);
    await page.getByText(t("材料明细","Material details"),{exact:true}).click();
    await expect(page.getByText(t("三钛合金 × 3","Tritanium × 3"),{exact:true})).toBeVisible();
    await page.getByLabel(t("税费 / %","Fees / %"),{exact:true}).fill("5");
    await expect(page.getByText("3 ISK/LP",{exact:true})).toBeVisible();
    await page.screenshot({path:info.outputPath(`lp-${language}.png`),fullPage:true});
    expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
    await page.getByLabel(t("搜索兑换物品","Search exchange items")).fill("");
    await page.getByRole("button",{name:t("下一页","Next page"),exact:true}).click();
    await expect(page.locator("tbody tr")).toHaveCount(1);
    await page.getByLabel(t("搜索兑换物品","Search exchange items")).fill("不存在的物品");
    await expect(page.getByText(t("没有匹配的兑换项目","No matching offers"),{exact:true})).toBeVisible();
  });
  test(`LP filters keyboard and empty corporation ${language}`,async({page})=>{
    await setup(page,language);await page.goto("/lp");
    await expect(page.getByText("3.2 ISK/LP",{exact:true})).toBeVisible();
    await page.getByRole("combobox",{name:t("NPC 阵营","NPC faction"),exact:true}).click();
    await page.getByRole("option",{name:t("加达里合众国","Caldari State"),exact:true}).click();
    await expect(page.getByRole("combobox",{name:t("NPC 军团","NPC corporation"),exact:true})).toContainText(t("加达里海军","Caldari Navy"));
    await expect(page.getByText(t("该军团暂无兑换项目","This corporation has no offers"),{exact:true})).toBeVisible();
    const market=page.getByRole("combobox",{name:t("市场","Market"),exact:true});await market.focus();await page.keyboard.press("Enter");await page.keyboard.press("Escape");await expect(market).toBeFocused();
    await page.getByLabel(t("税费 / %","Fees / %"),{exact:true}).fill("101");await expect(page.getByRole("alert")).toContainText(t("税费须为 0 至 100","Fees must be between 0 and 100"));
  });
  test(`LP price failure hides profit and can retry ${language}`,async({page})=>{
    await setup(page,language);await page.route("**/api/v1/lp/prices?**",r=>r.fulfill({status:503,json:{error:{message:"Market unavailable"}}}));
    await page.goto("/lp");await expect(page.getByRole("alert")).toContainText("Market unavailable");await expect(page.getByText("3.2 ISK/LP",{exact:true})).toHaveCount(0);
    await page.unroute("**/api/v1/lp/prices?**");await page.getByRole("button",{name:t("重试","Retry"),exact:true}).click();await expect(page.getByText("3.2 ISK/LP",{exact:true})).toBeVisible();
  });
}
