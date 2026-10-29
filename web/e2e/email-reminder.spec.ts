import { test, expect } from "@playwright/test";
import { english } from "../src/lib/locales/en";

for (const lang of ["zh-CN", "en"]) {
  test(`email configuration and verification ${lang}`, async ({ page }, info) => {
    const tr = (s: string) => lang === "en" ? english[s] ?? s : s;
    await page.addInitScript(l => localStorage.setItem("glorynavy.locale", l), lang);
    let settings = { enabled:false, host:"", port:465, implicit_tls:true, username:"", sender:"", secret_configured:false, revision:"1", events:[] as string[], available_events:[{id:"approval_pending", name:"待审批提醒"}] };
    let account = { email:"", verified:false, enabled:false, revision:"1", pending_email:"" };
    await page.route("https://images.evetech.net/**", route => route.abort());
    await page.route("**/api/v1/**", async route => {
      const req=route.request(); const path=new URL(req.url()).pathname; let data: unknown = {};
      if (path === "/api/v1/modules") data=["system","identity","eve","access","community","approval","reminder"].map(id => ({id,api_version:1,version:"0.1.0"}));
      if (path === "/api/v1/identity/session") data={ authenticated:true, session:{ user_id:"admin", character:{id:"101",name:"Pilot"}, csrf_token:"csrf", expires_at:"2099-01-01T00:00:00Z" } };
      if (path === "/api/v1/identity/characters") data={ characters:[{id:"101",name:"Pilot",status:"active",is_main:true}] };
      if (path === "/api/v1/eve/status") data={ configured:true };
      if (path === "/api/v1/access/me") data={ administrator:true,site_roles:[],characters:[] };
      if (path === "/api/v1/reminder/settings") data={ enabled:false,sender_id:"0",sender_name:"",revision:"1",status:"not_configured",characters:[],events:[],available_events:[] };
      if (path === "/api/v1/reminder/deliveries" || path === "/api/v1/reminder/email/deliveries") data=[];
      if (path === "/api/v1/reminder/email/settings") { if(req.method()==="PUT") { expect(req.headers()["x-csrf-token"]).toBe("csrf"); const { secret, ...next }=req.postDataJSON(); expect(secret).toBe("app-password"); settings={...settings,...next,revision:"2",secret_configured:true}; } data=settings; }
      if (path === "/api/v1/reminder/email/account") data=account;
      if (path === "/api/v1/reminder/email/challenge") { expect(req.headers()["x-csrf-token"]).toBe("csrf"); account={...account,pending_email:req.postDataJSON().address,revision:"2"}; data={sent:true}; }
      if (path === "/api/v1/reminder/email/verify") { expect(req.postDataJSON().code).toBe("12345678"); account={email:account.pending_email,verified:true,enabled:true,revision:"3",pending_email:""}; data=account; }
      await route.fulfill({json:{data}});
    });
    await page.goto("/reminders");
    const config=page.locator('[data-slot="card"]').filter({has:page.getByRole("heading",{name:tr("邮箱推送")})});
    await config.getByLabel(tr("SMTP 服务器")).fill("smtp.example.com");
    await config.getByLabel(tr("发件账号")).fill("sender@example.com");
    await config.getByLabel(tr("发件邮箱")).fill("sender@example.com");
    await config.getByLabel(tr("SMTP 授权码")).fill("app-password");
    await config.getByRole("checkbox",{name:tr("待审批提醒")}).check();
    await config.getByRole("checkbox",{name:tr("启用推送")}).check();
    await config.getByRole("button",{name:tr("保存"),exact:true}).click();
    await expect(config.getByText(tr("已启用"))).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.screenshot({path:info.outputPath(`email-settings-${lang}.png`),fullPage:true});
    await page.goto("/account");
    const email=page.locator('[data-slot="card"]').filter({has:page.getByRole("heading",{name:tr("邮箱提醒")})});
    await email.getByLabel(tr("收件邮箱")).fill("person@example.com");
    await email.getByRole("button",{name:tr("发送验证码")}).click();
    await email.getByLabel(tr("邮箱验证码")).fill("12345678");
    await email.getByRole("button",{name:tr("验证邮箱")}).click();
    await expect(email.getByText(/person@example.com/)).toBeVisible();
    await expect(email.getByText(tr("已验证"),{exact:false})).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.screenshot({path:info.outputPath(`email-${lang}.png`),fullPage:true});
  });
}
