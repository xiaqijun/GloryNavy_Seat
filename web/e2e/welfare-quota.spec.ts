import {test,expect,type Page} from '@playwright/test';

async function setup(page:Page,language:string,admin:boolean) {
 const user='00000000-0000-4000-8000-000000000001';
 const requests:Record<string,any>[]=[];
 let config: Record<string,any>={enabled:true,loss_rate_bps:10000,loss_daily_cap_minor:10000000000};
 await page.addInitScript(l=>localStorage.setItem('glorynavy.locale',l),language);
 await page.route('https://images.evetech.net/**',r=>r.abort());
 await page.route('**/api/v1/**',r=>{
  const p=new URL(r.request().url()).pathname;
  let data:unknown={};
  if(p==='/api/v1/modules') data=['system','identity','eve','access','exchange','welfare'].map(id=>({id,api_version:1,version:'0.1.0'}));
  else if(p==='/api/v1/identity/session') data={authenticated:true,session:{user_id:user,character:{id:'123',name:'Pilot'},csrf_token:'csrf',expires_at:'2099-01-01T00:00:00Z'}};
  else if(p==='/api/v1/access/me') data={administrator:admin,can_manage:admin,can_manage_sync:false,site_roles:[],characters:[]};
  else if(p==='/api/v1/welfare/context') data={
   administrator:admin,corporations:[{id:'10',name:'Glory Navy',can_manage:admin,can_compensate:admin}],characters:[{id:'123',name:'Pilot',account_id:user,corporation_id:'10'}],
   policies:[{corporation_id:'10',kind:'srp',version:'1',config}],
   loss_quotas:[{kind:'srp',daily:{used_minor:10000000000,reserved_minor:0,cap_minor:10000000000,remaining_minor:0},weekly:{used_minor:25000000000,reserved_minor:10000000000},monthly:{used_minor:25000000000,reserved_minor:10000000000},future_used_minor:15000000000,future_reserved_minor:10000000000},
    {kind:'solo',daily:{used_minor:0,reserved_minor:0},weekly:{used_minor:0,reserved_minor:0},monthly:{used_minor:0,reserved_minor:0},future_used_minor:0,future_reserved_minor:0}],
  };
  else if(p==='/api/v1/welfare/commands') {requests.push(r.request().postDataJSON());config=requests.at(-1)!.config;data={saved:true};}
  else if(p.startsWith('/api/v1/welfare/')) data={items:[],next_cursor:''};
  return r.fulfill({json:{data}});
 });
 await page.goto('/welfare');
 return requests;
}

for(const language of ['zh-CN','en']) {
 test(`${language} future daily usage and pending reservations`,async({page},info)=>{
  await setup(page,language,false);
  const future=page.getByLabel(language==='en'?'Future daily allowance':'后续日额度',{exact:true});
  await expect(future).toBeVisible();
  await expect(future).toContainText('150,000,000');
  await expect(future).toContainText('100,000,000');
  await expect(future).toHaveCount(1);
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=document.documentElement.clientWidth)).toBe(true);
  await page.screenshot({path:`../.local/loss-quota/summary-${language}-${info.project.name}.png`,fullPage:true});
 });
 test(`${language} quota configuration retains weekly and monthly ceilings`,async({page},info)=>{
  const requests=await setup(page,language,true);
  await page.getByRole('button',{name:language==='en'?'Reimbursement settings':'补损设置',exact:true}).click();
  const daily=page.getByLabel(language==='en'?'Daily allowance per member / ISK':'每人每日额度 / ISK');
  await expect(daily).toHaveValue('100000000');
  await daily.fill('200000000');
  await page.getByLabel(language==='en'?'Weekly cap per account / ISK':'每人每周上限 / ISK').fill('600000000');
  await page.getByLabel(language==='en'?'Monthly cap per account / ISK':'每人每月上限 / ISK').fill('2000000000');
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=document.documentElement.clientWidth)).toBe(true);
  await page.screenshot({path:`../.local/loss-quota/settings-${language}-${info.project.name}.png`,fullPage:true});
  await page.getByRole('button',{name:language==='en'?'Save':'保存',exact:true}).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  expect(requests[0]).toMatchObject({action:'configure',config:{loss_daily_cap_minor:20000000000,loss_weekly_cap_minor:60000000000,loss_monthly_cap_minor:200000000000}});
 });
}
for (const language of ['zh-CN','en']) {
 test(`${language} daily allowance fills automatic caps and preserves manual edits`,async({page},info)=>{
  const requests=await setup(page,language,true);
  const open=()=>page.getByRole('button',{name:language==='en'?'Reimbursement settings':'补损设置',exact:true}).click();
  const daily=page.getByLabel(language==='en'?'Daily allowance per member / ISK':'每人每日额度 / ISK');
  const weekly=page.getByLabel(language==='en'?'Weekly cap per account / ISK':'每人每周上限 / ISK');
  const monthly=page.getByLabel(language==='en'?'Monthly cap per account / ISK':'每人每月上限 / ISK');
  const save=page.getByRole('button',{name:language==='en'?'Save':'保存',exact:true});
  await open();
  await expect(weekly).toHaveValue('700000000');
  await expect(monthly).toHaveValue('3000000000');
  await daily.fill('200000000');
  await expect(weekly).toHaveValue('1400000000');
  await expect(monthly).toHaveValue('6000000000');
  await weekly.fill('900000000');
  await daily.fill('300000000');
  await expect(weekly).toHaveValue('900000000');
  await expect(monthly).toHaveValue('9000000000');
  await monthly.fill('8000000000');
  await daily.fill('200000000');
  await expect(monthly).toHaveValue('8000000000');
  await monthly.fill('0');
  await expect(monthly).toHaveValue('6000000000');
  await weekly.fill('');
  await expect(weekly).toHaveValue('1400000000');
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=document.documentElement.clientWidth)).toBe(true);
  await page.screenshot({path:`../.local/loss-quota-defaults/auto-${language}-${info.project.name}.png`,fullPage:true});
  await save.click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  expect(requests[0]).toMatchObject({config:{loss_daily_cap_minor:20000000000,loss_weekly_cap_minor:0,loss_monthly_cap_minor:0}});
  await open();
  await expect(weekly).toHaveValue('1400000000');
  await daily.fill('1000000000000');
  await expect(weekly).toHaveValue('7000000000000');
  await expect(monthly).toHaveValue('30000000000000');
  await weekly.fill('1');
  await weekly.fill('7000000000000');
  await monthly.fill('1');
  await monthly.fill('30000000000000');
  await save.click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  expect(requests[1]).toMatchObject({config:{loss_daily_cap_minor:100000000000000,loss_weekly_cap_minor:700000000000000,loss_monthly_cap_minor:3000000000000000}});
 });
}
