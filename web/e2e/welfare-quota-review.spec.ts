import { test, expect, type Page } from "@playwright/test";
const admin = "00000000-0000-4000-8000-000000000001",
  member = "00000000-0000-4000-8000-000000000002";
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
    killmail_id: "139267042",
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
      id: "139267042",
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

const report = { status: "ready", summary: {
 start: "2026-10-18T20:07:18Z", end: "2026-10-18T20:40:51Z", observed_at: "2026-10-19T02:00:00Z", systems: ["15W-GC"], kill_count: 80, total_isk: 8267242054.6,
 teams: [{ id: "A", name: "FRT+F.A.X+NC +6", pilots: 44, kills: 55, losses: 25, kills_isk: 4600117203.74, losses_isk: 3667124850.86, own: true },
 { id: "B", name: "INIT.", pilots: 72, kills: 24, losses: 55, kills_isk: 3667114850.86, losses_isk: 4600117203.74, own: false }] } };
async function setup(page:Page, language:string, review:boolean, status="ready") {
 const reads:string[]=[];
 const commands:Record<string,any>[]=[];
 let quotaLevel=status;
 let fail=false;
 let cutoff="";
  let reportStatus='ready';
 await page.addInitScript(l=>localStorage.setItem('glorynavy.locale',l),language);
 await page.route('https://images.evetech.net/**',r=>r.abort());
 await page.route('**/api/v1/**',r=>{
  const u=new URL(r.request().url()),p=u.pathname;
  reads.push(p+u.search);
  let data:unknown={};
  if(p==='/api/v1/modules') data=['system','identity','eve','access','exchange','welfare',...(review?['approval']:[])].map(id=>({id,api_version:1,version:'0.1.0'}));
  else if(p==='/api/v1/identity/session') data={authenticated:true,session:{user_id:review?admin:member,character:{id:'123',name:'Loss Pilot'},csrf_token:'csrf',expires_at:'2099-01-01T00:00:00Z'}};
  else if(p==='/api/v1/access/me') data={administrator:review,can_manage:review,can_manage_sync:false,site_roles:[],characters:[]};
  else if(p==='/api/v1/welfare/context') data={administrator:review,corporations:[{id:'10',name:'Glory Navy',can_manage:review,can_compensate:review}],characters:[{id:'123',name:'Loss Pilot',account_id:review?admin:member,corporation_id:'10'}],policies:[{corporation_id:'10',kind:'solo',version:'1',config:{...rule,project_name:'',loss_daily_cap_minor:1000000000,loss_weekly_cap_minor:10000000000,loss_monthly_cap_minor:30000000000,loss_occurred_after:cutoff}}]};
  else if(p==='/api/v1/approval/context') data={allowed:true,sources:['welfare'],corporations:[{id:'10',name:'Glory Navy'}],people:[{id:member,name:'Loss Pilot'}],unavailable:[]};
  else if(p==='/api/v1/approval/items') data={items:[lossRow],counts:{pending:1,fulfillment:0,exceptions:0,history:0},next_cursor:'',unavailable:[]};
  else if(p==='/api/v1/approval/items/welfare/9') data=lossRow;
  else if(p.endsWith('/loss-quota')) {
   if(quotaLevel==='error')return r.fulfill({status:503,json:{error:{message:'额度读取失败'}}});
   const period=(cap:number,left:number)=>({used_minor:cap-left,reserved_minor:0,cap_minor:cap,remaining_minor:left});
   data={case_version:'2',policy_version:'1',checked_at:'2026-10-19T00:00:00Z',quota:{kind:'solo',daily:period(1000000000,1000000000),weekly:period(10000000000,quotaLevel==='weekly'?10000000000:1000000000),monthly:period(30000000000,quotaLevel==='blocked'?1000000000:30000000000)}};
  }
  else if(p==='/api/v1/welfare/commands') {
   const cmd=r.request().postDataJSON();commands.push(cmd);
   if(fail){fail=false;quotaLevel='monthly';return r.fulfill({status:409,json:{error:{message:'周额度不足，请确认使用月额度'}}});}
   if(cmd.action==='configure')cutoff=cmd.config.loss_occurred_after;
   data={ok:true};
  }
  else if(p.endsWith('/battle-report')) data=reportStatus==='ready'?report:{status:reportStatus};
  else if(p==='/api/v1/welfare/cases/9') data={item:lossItem,history:[]};
  else if(p==='/api/v1/welfare/cases') data={items:[lossItem],next_cursor:''};
  else if(p==='/api/v1/welfare/losses') data={items:[{...lossItem.detail.loss_evidence,reimbursement:{status:'available',state:'',kind:'',available_kinds:['solo']}}],next_cursor:''};
  else if(p==='/api/v1/welfare/losses/prices') data={items:[]};
  return r.fulfill({json:{data}});
 });
 await page.goto(review?'/approvals?source=welfare&id=9':'/welfare');
 return {reads,commands,setQuota:(s:string)=>{quotaLevel=s;},failNext:()=>{fail=true;},setCutoff:(s:string)=>{cutoff=s;}};
}
for(const language of ['zh-CN','en']) {
 const en=language==='en';
 test(`${language} weekly consent is explicit and amount changes reset it`,async({page},info)=>{
  const ctl=await setup(page,language,true,'weekly');
  await page.getByRole('button',{name:en?'Approve':'批准',exact:true}).click();
  await page.getByLabel(en?'Action note':'处理说明',{exact:true}).fill('Reviewed');
  const submit=page.getByRole('button',{name:en?'Confirm':'确认',exact:true});
  const consent=page.getByRole('checkbox',{name:en?'Confirm use of weekly quota':'确认使用周额度'});
  await expect(consent).not.toBeChecked();await expect(submit).toBeDisabled();
  await consent.check();await expect(submit).toBeEnabled();
  await page.getByRole('checkbox',{name:en?'Manual appraisal':'人工核价',exact:true}).check();
  await expect(consent).not.toBeChecked();
  await page.getByLabel(en?'Assessed value / ISK':'核价金额 / ISK',{exact:true}).fill('40000000');
  await consent.check();
  const panel=page.getByRole('region',{name:en?'Quota confirmation':'额度确认'});
  expect(await panel.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true);
  await panel.scrollIntoViewIfNeeded();await page.screenshot({path:`../.local/quota-confirm/weekly-${language}-${info.project.name}.png`,fullPage:true});
  await submit.click();await expect.poll(()=>ctl.commands.length).toBe(1);
  expect(ctl.commands[0]).toMatchObject({action:'approve',loss_quota_consent:'weekly',loss_policy_version:'1',detail:{base_minor:4000000000}});
 });
 test(`${language} concurrent consumption requires new monthly confirmation`,async({page})=>{
  const ctl=await setup(page,language,true,'weekly');ctl.failNext();
  await page.getByRole('button',{name:en?'Approve':'批准',exact:true}).click();
  await page.getByLabel(en?'Action note':'处理说明',{exact:true}).fill('Reviewed');
  await page.getByRole('checkbox',{name:en?'Confirm use of weekly quota':'确认使用周额度'}).check();
  const submit=page.getByRole('button',{name:en?'Confirm':'确认',exact:true});await submit.click();
  const monthly=page.getByRole('checkbox',{name:en?'Confirm use of monthly quota':'确认使用月额度'});
  await expect(monthly).not.toBeChecked();await expect(submit).toBeDisabled();
  await monthly.check();await submit.click();await expect.poll(()=>ctl.commands.length).toBe(2);
  expect(ctl.commands[1].loss_quota_consent).toBe('monthly');
 });
 test(`${language} exhausted month and read failure block approval`,async({page})=>{
  const ctl=await setup(page,language,true,'error');
  await page.getByRole('button',{name:en?'Approve':'批准',exact:true}).click();
  const submit=page.getByRole('button',{name:en?'Confirm':'确认',exact:true});await expect(submit).toBeDisabled();
  ctl.setQuota('blocked');
  await page.getByRole('button',{name:en?'Retry':'重试',exact:true}).click();
  await expect(page.getByText(en?'Monthly quota insufficient. Approval unavailable.':'月额度不足，无法批准',{exact:true})).toBeVisible();
  await expect(submit).toBeDisabled();expect(ctl.commands).toHaveLength(0);
 });
 test(`${language} start time setting persists in Beijing time`,async({page},info)=>{
  const ctl=await setup(page,language,true,'weekly');await page.goto('/welfare');
  await page.getByRole('button',{name:en?'Reimbursement settings':'补损设置',exact:true}).click();
  const start=page.getByLabel(en?'Earliest eligible loss (Beijing time)':'可申请起始时间（北京时间）');
  await start.fill('2026-10-01T00:00');
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
  await page.screenshot({path:`../.local/quota-confirm/settings-${language}-${info.project.name}.png`,fullPage:true});
  await page.getByRole('button',{name:en?'Save':'保存',exact:true}).click();
  await expect.poll(()=>ctl.commands.length).toBe(1);
  expect(ctl.commands[0].config.loss_occurred_after).toBe('2026-09-30T16:00:00.000Z');
 });
 test(`${language} older loss cannot be submitted`,async({page})=>{
  const ctl=await setup(page,language,false,'weekly');ctl.setCutoff('2026-10-01T00:00:00+08:00');
  await page.goto('/losses');await page.locator('.welfare-loss-row').click();
  await page.getByRole('button',{name:en?'Apply for SRP':'申请补损',exact:true}).click();
  await expect(page.getByText(en?'The loss occurred before the application start time.':'损失时间早于可申请起始时间',{exact:false})).toBeVisible();
  await expect(page.getByRole('button',{name:en?'Submit application':'提交申请',exact:true})).toBeDisabled();
  expect(ctl.commands).toHaveLength(0);
 });
}
