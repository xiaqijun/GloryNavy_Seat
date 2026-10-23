import { test,expect } from "@playwright/test";

for(const language of ["zh-CN","en"]){
 test(`feedback complete lifecycle ${language}`,async({page},info)=>{
  const en=language==="en",owner="00000000-0000-4000-8000-000000000001",admin="00000000-0000-4000-8000-000000000002";
  let staff=true,rewards=0;
  const ticket:any={id:"1",account_id:owner,author_name:"Submitter",kind:"feature",title:"完整流程测试",body:"希望改进功能",status:"submitted",revision:1,created_at:"2026-10-23T00:00:00Z",updated_at:"2026-10-23T00:00:00Z"};
  const messages:any[]=[];
  await page.addInitScript(l=>localStorage.setItem("glorynavy.locale",l),language);
  await page.route("https://images.evetech.net/**",r=>r.abort());
  await page.route("**/api/v1/**",async r=>{
   const p=new URL(r.request().url()).pathname;let data:any={};
   if(p==="/api/v1/modules")data=["system","identity","eve","access","feedback"].map(id=>({id,api_version:1,version:"0.1.0"}));
   if(p==="/api/v1/identity/session")data={authenticated:true,session:{user_id:staff?admin:owner,character:{id:"123",name:staff?"Admin":"Submitter"},csrf_token:"csrf",expires_at:"2099-01-01T00:00:00Z"}};
   if(p==="/api/v1/access/me")data={administrator:staff,can_manage:staff,can_manage_sync:false,site_roles:[],characters:[]};
   if(p==="/api/v1/feedback/tickets")data={tickets:[ticket],next_before:"",administrator:staff};
   if(p==="/api/v1/feedback/tickets/1")data={ticket,messages,administrator:staff};
   if(p.endsWith("/messages")){
    const input=r.request().postDataJSON();expect(input.revision).toBe(ticket.revision);expect(input).not.toHaveProperty("reward_minor");
    expect(r.request().headers()["x-csrf-token"]).toBe("csrf");ticket.status=input.status;ticket.revision++;
    if(input.status==="accepted"&&!ticket.accepted_at){ticket.accepted_at=ticket.updated_at;ticket.reward_minor=2500;rewards++;}
    messages.push({id:String(messages.length+1),author_name:staff?"Admin":"Submitter",staff,body:input.body,status:input.status,created_at:ticket.updated_at});data=ticket;
   }
   await r.fulfill({json:{data}});
  });
  const act=async(zh:string,english:string,note?:string)=>{
   const dialog=page.getByRole("dialog");await dialog.getByRole("button",{name:en?english:zh,exact:true}).click();
   const save=dialog.getByRole("button",{name:en?"Confirm action":"确认操作",exact:true});
   if(note!==undefined){await expect(save).toBeDisabled();await dialog.getByRole("textbox",{name:en?"Result or reason":"处理结果或原因",exact:true}).fill(note);}
   if(zh==="提交完成")await page.screenshot({path:info.outputPath(`review-${language}.png`),fullPage:true});
   await save.click();await expect(save).toHaveCount(0);
  };
  const open=async()=>{await page.goto("/feedback?ticket=1");await expect(page.getByRole("dialog")).toBeVisible();};
  await open();await page.screenshot({path:info.outputPath(`submitted-${language}.png`),fullPage:true});await act("采纳反馈","Accept feedback");await act("开始处理","Start handling");await act("提交完成","Submit completion","已经处理，请确认");
  await expect(page.getByRole("button",{name:en?"Confirm completion":"确认完成",exact:true})).toHaveCount(0);
  staff=false;await open();await act("退回处理","Return for handling","结果不符合预期");
  await expect(page.getByRole("button",{name:en?"Accept feedback":"采纳反馈",exact:true})).toHaveCount(0);
  staff=true;await open();await act("提交完成","Submit completion","已补充修复");
  staff=false;await open();await act("确认完成","Confirm completion");
  await expect(page.locator('.feedback-progress [aria-current="step"]')).toContainText(en?"Finished":"已完成");
  expect(rewards).toBe(1);expect(messages.map(m=>m.status)).toEqual(["accepted","in_progress","awaiting_confirmation","in_progress","awaiting_confirmation","resolved"]);
  await expect(page.getByText("25",{exact:false}).first()).toBeVisible();
  await page.screenshot({path:info.outputPath(`completed-${language}.png`),fullPage:true});
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
 });
}
