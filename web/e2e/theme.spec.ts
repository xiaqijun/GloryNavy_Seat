import { test, expect, type Page } from "@playwright/test";

async function setup(page: Page, locale = "zh-CN") {
  await page.addInitScript((locale) => localStorage.setItem("glorynavy.locale", locale), locale);
  await page.route("https://images.evetech.net/**", (route) => route.abort());
  await page.route("**/api/v1/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    let data: unknown = {};
    if (path === "/api/v1/modules") data = ["system", "identity", "eve", "access", "feedback"].map((id) => ({ id, api_version: 1, version: "0.1.0" }));
    if (path === "/api/v1/identity/session") data = { authenticated: true, session: {
      user_id: "00000000-0000-4000-8000-000000000001", character: { id: "123", name: "Pilot" },
      csrf_token: "csrf", expires_at: "2099-01-01T00:00:00Z",
    } };
    if (path === "/api/v1/access/me") data = { administrator: true, can_manage: true, characters: [], site_roles: [] };
    if (path === "/api/v1/feedback/tickets") data = { tickets: [], next_before: "", administrator: true };
    await route.fulfill({ json: { data } });
  });
}

async function select(page: Page, name: string) {
  await page.getByRole("combobox", { name: /^(主题|Theme)$/ }).click();
  await page.getByRole("option", { name, exact: true }).click();
}

test("theme follows the system and explicit preferences survive navigation and reload", async ({ page }) => {
  await setup(page);
  await page.emulateMedia({ colorScheme: "dark" });
  await page.goto("/feedback");
  const html = page.locator("html");
  await expect(html).toHaveAttribute("data-theme", "dark");
  await select(page, "浅色");
  await expect(html).toHaveAttribute("data-theme", "light");
  await page.emulateMedia({ colorScheme: "light" });
  await page.emulateMedia({ colorScheme: "dark" });
  await expect(html).toHaveAttribute("data-theme", "light");
  await page.reload();
  await expect(html).toHaveAttribute("data-theme", "light");
  await select(page, "暗色");
  await expect(html).toHaveAttribute("data-theme", "dark");
  await page.reload();
  await expect(html).toHaveAttribute("data-theme", "dark");
  await select(page, "跟随系统");
  await page.emulateMedia({ colorScheme: "light" });
  await expect(html).toHaveAttribute("data-theme", "light");
  await page.emulateMedia({ colorScheme: "dark" });
  await expect(html).toHaveAttribute("data-theme", "dark");
});

for (const locale of ["zh-CN", "en"]) {
  test(`theme controls and dark dialogs keep the form draft ${locale}`, async ({ page }, info) => {
    await setup(page, locale);
    const en = locale === "en";
    await page.goto("/feedback");
    const theme = page.getByRole("combobox", { name: en ? "Theme" : "主题", exact: true });
    await theme.focus();
    await page.keyboard.press("Enter");
    await page.getByRole("option", { name: en ? "Dark" : "暗色", exact: true }).click();
    await expect(theme).toBeFocused();
    await page.getByRole("button", { name: en ? "Submit feedback" : "提交反馈", exact: true }).click();
    const dialog = page.getByRole("dialog");
    const title = dialog.getByLabel(en ? "Title" : "标题", { exact: true });
    await title.fill("Theme draft — 保留输入");
    await page.emulateMedia({ colorScheme: "light" });
    // Cross-tab updates must repaint without a document reload or lost draft.
    await page.evaluate(() => {
      localStorage.setItem("glorynavy.theme", "light");
      dispatchEvent(new StorageEvent("storage", { key: "glorynavy.theme", storageArea: localStorage }));
    });
    await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
    await expect(title).toHaveValue("Theme draft — 保留输入");
    await page.evaluate(() => {
      localStorage.setItem("glorynavy.theme", "dark");
      dispatchEvent(new StorageEvent("storage", { key: "glorynavy.theme", storageArea: localStorage }));
    });
    await expect(title).toHaveValue("Theme draft — 保留输入");
    await expect(dialog).toHaveCSS("background-color", "rgb(22, 34, 53)");
    await expect(title).toHaveCSS("color", "rgb(226, 232, 240)");
    await dialog.getByRole("combobox").click();
    await expect(page.getByRole("listbox")).toHaveCSS("background-color", "rgb(22, 34, 53)");
    await page.keyboard.press("Escape");
    await expect(dialog).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.screenshot({ path: info.outputPath(`dark-form-${locale}.png`), fullPage: true });
  });
}

test("saved theme runs before React", async ({ page }) => {
  await setup(page);
  await page.addInitScript(() => localStorage.setItem("glorynavy.theme", "dark"));
  await page.route("**/assets/*.js", (route) => route.abort());
  await page.emulateMedia({ colorScheme: "light" });
  await page.goto("/", { waitUntil: "domcontentloaded" });
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await expect(page.locator("html")).not.toHaveAttribute("data-app-ready");
});

test("privacy settings fall back to the system", async ({ page }) => {
  await setup(page);
  await page.addInitScript(() => {
    Object.defineProperty(Storage.prototype, "getItem", { value() { throw new DOMException("Blocked", "SecurityError"); } });
    Object.defineProperty(Storage.prototype, "setItem", { value() { throw new DOMException("Blocked", "SecurityError"); } });
  });
  await page.emulateMedia({ colorScheme: "dark" });
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("/feedback");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await select(page, "浅色");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  expect(errors).toEqual([]);
});

test("real browser tabs synchronize preferences and invalid values use the system", async ({ page, context }) => {
  await setup(page);
  await page.emulateMedia({ colorScheme: "light" });
  await page.goto("/feedback");
  const other = await context.newPage();
  await setup(other);
  await other.goto("/feedback");
  await select(other, "暗色");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await other.evaluate(() => localStorage.setItem("glorynavy.theme", "invalid"));
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  await page.bringToFront();
  await select(page, "暗色");
  await other.evaluate(() => localStorage.clear());
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  await other.close();
});

test("public home uses the same preference with readable dark content sections", async ({ page }, info) => {
  await setup(page);
  await page.emulateMedia({ colorScheme: "dark" });
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("/");
  await expect(page.getByRole("combobox", { name: "主题", exact: true })).toBeVisible();
  await expect(page.locator(".landing-introduction")).toHaveCSS("background-color", "rgb(11, 18, 32)");
  await page.locator("#strength").scrollIntoViewIfNeeded();
  await page.screenshot({ path: info.outputPath("dark-home.png"), fullPage: false });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  expect(errors).toEqual([]);
});

test("existing PAP chart repaints immediately when the theme changes", async ({ page }, info) => {
  await setup(page);
  await page.emulateMedia({ colorScheme: "dark", reducedMotion: "reduce" });
  await page.route("**/api/v1/modules", (route) => route.fulfill({ json: { data:
    ["system", "identity", "eve", "access", "attendance"].map((id) => ({ id, api_version: 1, version: "0.1.0" })),
  } }));
  await page.route("**/api/v1/attendance/pap-requirement", (route) => route.fulfill({ json: { data: {
    source: "alliance", monthly_points: 3, version: "1", can_manage: false,
  } } }));
  await page.route("**/api/v1/attendance/alliance-pap", (route) => route.fulfill({ json: { data: {
    source: "alliance", month: "2026-10", points: 1, available: true, complete: true,
    state: "ready", records_total: 1, last_synced_at: "2026-10-23T00:00:00Z",
    characters: [], version: "1", can_manage: false,
  } } }));
  await page.goto("/workspace");
  const ring = page.locator(".ring-progress");
  await expect(ring.locator('path[fill="#60a5fa"]')).toHaveCount(1);
  await select(page, "浅色");
  await expect(ring.locator('path[fill="#2563eb"]')).toHaveCount(1);
  await select(page, "暗色");
  await expect(ring.locator('path[fill="#60a5fa"]')).toHaveCount(1);
  await expect(page.locator(".ring-progress-value")).toHaveText("1");
  await page.screenshot({ path: info.outputPath("dark-chart.png"), fullPage: true });
});

test("theme text tokens retain readable contrast on their matching surfaces", async ({ page }) => {
  await setup(page);
  await page.goto("/feedback");
  for (const mode of ["暗色", "浅色"]) {
    await select(page, mode);
    const contrasts = await page.evaluate(() => {
      const style = getComputedStyle(document.documentElement);
      const light = (token: string) => {
        const hex = style.getPropertyValue(token).trim().slice(1);
        const full = hex.length === 3 ? [...hex].map((c) => c + c).join("") : hex;
        const rgb = [0, 2, 4].map((i) => parseInt(full.slice(i, i + 2), 16) / 255)
          .map((c) => c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4);
        return rgb[0] * 0.2126 + rgb[1] * 0.7152 + rgb[2] * 0.0722;
      };
      return [
        ["--foreground", "--card"], ["--muted", "--card"],
        ["--muted", "--background"], ["--text-secondary", "--surface-muted"],
        ["--primary-foreground", "--primary"], ["--primary-hover", "--info-surface"],
        ["--danger-text", "--danger-surface"], ["--success", "--success-surface"],
        ["--warning-text", "--warning-surface"],
      ].map(([text, background]) => {
        const a = light(text), b = light(background);
        return { text, background, ratio: (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05) };
      });
    });
    for (const pair of contrasts) expect(pair.ratio, `${mode}: ${pair.text}/${pair.background}`).toBeGreaterThanOrEqual(4.5);
  }
});
