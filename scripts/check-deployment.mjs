// Read-only endpoints plus one real login initiation; no player credentials used.
import { createRequire } from 'node:module';
const require = createRequire(new URL('../web/package.json', import.meta.url));
const { chromium } = require('@playwright/test');
const origin = process.argv[2];
if (!origin || new URL(origin).origin !== origin || !origin.startsWith('https://')) {
  throw new Error('Usage: node scripts/check-deployment.mjs https://your-domain');
}
const browser = await chromium.launch({ headless: true,
  executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE || undefined });
try {
  const page = await browser.newPage();
  for (const path of ['/api/v1/modules','/api/v1/system/status','/api/v1/eve/status']) {
    const response = await page.request.get(origin + path);
    if (response.status() !== 401) throw new Error('Anonymous metadata exposed: ' + path);
  }
  const loginStatus = (await (await page.request.get(origin + '/api/v1/eve/login-status')).json()).data;
  if (Object.keys(loginStatus).join(',') !== 'configured') throw new Error('Login projection expanded');
  const errors = [];
  page.on('pageerror', error => {
    // CCP may serve its own challenge page after the expected SSO redirect.
    // Only a script error on this site is a deployment regression.
    if (new URL(page.url()).origin === origin) errors.push(error.message);
  });
  // Exercise the real form and production API; stop before CCP player sign-in.
  await page.route('https://login.eveonline.com/**', route => route.fulfill({
    status: 200, contentType: 'text/html', body: '<title>SSO redirect reached</title>',
  }));
  const document = await page.goto(origin + '/', { waitUntil: 'networkidle' });
  if (document.status() !== 200) throw new Error('Public home unavailable');
  await page.getByRole('heading', { name: 'GloryNavy', exact: true }).waitFor();
  if (await page.locator('.sidebar, .desk-review').count()) throw new Error('Private workspace mounted on public home');
  const responsePromise = page.waitForResponse(response =>
    response.url() === origin + '/api/v1/eve/login' && response.request().method() === 'POST');
  await page.getByRole('button', { name: '使用 EVE Online 登录' }).click();
  const response = await responsePromise;
  const requestHeaders = await response.request().allHeaders();
  if (requestHeaders.origin !== origin || response.status() !== 303) {
    throw new Error(`Browser form rejected: HTTP ${response.status()}, matching Origin=${requestHeaders.origin === origin}`);
  }
  const headers = await response.allHeaders();
  const location = new URL(headers.location);
  const fittingWriteEnabled = location.searchParams.get('scope').split(' ').includes('esi-fittings.write_fittings.v1');
  if (location.origin !== 'https://login.eveonline.com' ||
      location.searchParams.get('redirect_uri') !== origin + '/api/v1/eve/callback' ||
      location.searchParams.get('code_challenge_method') !== 'S256' ||
      location.searchParams.get('scope').split(' ').length !== (fittingWriteEnabled ? 58 : 57) ||
      location.searchParams.get('scope').split(' ').includes('esi-fittings.write_fittings.v1') !== fittingWriteEnabled) {
    throw new Error('Unexpected SSO redirect parameters');
  }
  if (headers['referrer-policy'] !== 'no-referrer') throw new Error('API referrer policy weakened');
  const cookies = await page.context().cookies(origin);
  const flow = cookies.find(cookie => cookie.name === 'gn_eve_flow');
  if (!flow?.secure || !flow.httpOnly || flow.sameSite !== 'Lax') throw new Error('Flow cookie protection missing');
  for (const rejectedOrigin of ['null', 'https://untrusted.example']) {
    const denied = await page.request.post(origin + '/api/v1/eve/login', {
      headers: { Origin: rejectedOrigin }, maxRedirects: 0,
    });
    if (denied.status() !== 403) throw new Error('Untrusted Origin was accepted');
  }
  const privateResponse = await page.request.get(origin + '/api/v1/eve/contracts/owners');
  if (privateResponse.status() !== 401 || errors.length) {
    throw new Error(`Browser or authorization check failed: HTTP ${privateResponse.status()}, page errors: ${JSON.stringify(errors)}`);
  }
  console.log('PASS: public home, real browser form POST, matching Origin, 303 SSO redirect, callback/PKCE/scopes, secure cookie, API no-referrer, null/cross-site rejection, anonymous isolation');
} finally {
  await browser.close();
}
