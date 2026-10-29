import { test, expect } from "@playwright/test";
import { english } from "../src/lib/locales/en";

for (const language of ["zh-CN", "en"]) {
  test(`approval requires current repayment risk acknowledgement ${language}`, async ({page}, info) => {
    const tr = (s: string) => language === "en" ? english[s] ?? s : s;
    const loan = {id:"1",public_id:"LN-1",pool_id:"1",pool_name:"Pool",lender_kind:"corporation",borrower_account_id:"borrower",borrower_character_id:"111",principal_minor:100000,interest_minor:0,total_due_minor:100000,installment_count:1,interval_days:30,state:"submitted",version:1,created_at:"2026-10-29T00:00:00Z"};
    const row = {source:"loan",id:"1",version:"1",account_id:"borrower",applicant:"Pilot",corporation_id:"10",kind:"loan",state:"submitted",status:"submitted",recipient:"Pilot",title:"Pool",reference:"LN-1",amount_minor:100000,unit:"isk",time:loan.created_at,action:"",actions:["approve","reject"],payload:loan};
    let writes=0;
    await page.addInitScript(l => localStorage.setItem("glorynavy.locale",l),language);
    await page.route("**/api/v1/**", async r => {
      const p=new URL(r.request().url()).pathname; let data: unknown={};
      if(p==="/api/v1/modules") data=["system","identity","eve","access","loan","approval"].map(id=>({id,version:"0.1.0",api_version:1}));
      else if(p==="/api/v1/identity/session") data={authenticated:true,session:{user_id:"manager",character:{id:"222",name:"Reviewer"},csrf_token:"csrf",expires_at:"2099-01-01T00:00:00Z"}};
      else if(p==="/api/v1/access/me") data={administrator:true,can_manage:true,site_roles:[],characters:[]};
      else if(p==="/api/v1/approval/context") data={allowed:true,sources:["loan"],corporations:[{id:"10",name:"Corp"}],people:[],unavailable:[]};
      else if(p==="/api/v1/approval/items") data={items:[row],counts:{pending:1,fulfillment:0,exceptions:0,history:0},next_cursor:"",unavailable:[]};
      else if(p==="/api/v1/approval/items/loan/1") data=row;
      else if(p==="/api/v1/loan/cases/1") data={case:loan,installments:[],guarantees:[],collateral:[],repayment_risk:{status:"insufficient",days:30,estimated_capacity_minor:30000,total_due_minor:100000,token:"risk-token"}};
      else if(p==="/api/v1/loan/cases/1/review") {writes++; expect(r.request().headers()["x-csrf-token"]).toBe("csrf"); expect(r.request().postDataJSON().risk_acknowledgement).toBe("risk-token"); data=loan;}
      await r.fulfill({json:{data}});
    });
    await page.goto("/approvals?view=pending");
    await page.locator(".approval-page").getByRole("button",{name:/LN-1|贷款|Loan/}).first().click();
    const dialog=page.getByRole("dialog");
    await expect(dialog.getByText(tr("预计还款能力不足，请核对风险后决定。"))).toBeVisible();
    const approve=dialog.getByRole("button",{name:tr("批准"),exact:true});
    await expect(approve).toBeDisabled();
    await dialog.getByRole("checkbox",{name:tr("已了解还款风险")}).check();
    await expect(approve).toBeEnabled();
    expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
    await page.screenshot({path:info.outputPath(`loan-risk-${language}.png`),fullPage:true});
    await approve.click();
    expect(writes).toBe(1);
  });
}
