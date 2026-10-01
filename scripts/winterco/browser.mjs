import { fileURLToPath } from "node:url";
import { mkdir, writeFile, rename } from "node:fs/promises";
import { createHash } from "node:crypto";
import { existsSync } from "node:fs";
import { spawn } from "node:child_process";
import path from "node:path";
import { chromium } from "../../web/node_modules/playwright/index.mjs";

export const origin = "https://seat.winterco.org";
export function papURL(character) {
  if (!/^[1-9]\d{0,15}$/.test(character))
    throw new Error("Invalid character ID");
  return `${origin}/character/view/paps/${character}`;
}

export function classifyPage(url, character, status) {
  const parsed = new URL(url);
  if (
    parsed.origin === "https://login.eveonline.com" ||
    (parsed.origin === origin && parsed.pathname.startsWith("/auth/"))
  )
    return "login_required";
  if (parsed.origin !== origin) return "unexpected_destination";
  if (status === 401) return "login_required";
  if (status === 403) return "permission_denied";
  if (status === 429) return "rate_limited";
  if (status !== undefined && status !== 200) return "upstream_error";
  if (parsed.href !== papURL(character)) return "unexpected_destination";
  return "inspectable";
}

// One dedicated local profile; never copy cookies from an existing personal browser.
export async function openBrowser(directory, channel, headless) {
  await mkdir(directory, { recursive: true, mode: 0o700 });
  return chromium.launchPersistentContext(
    fileURLToPath(new URL("profile/", directory)),
    {
      channel,
      headless,
      acceptDownloads: false,
    },
  );
}

// Login is a user-operated, normal browser launch. No automation or debugging
// flags, credential entry, personal-profile copying or anti-detection patches.
export function manualLoginArgs(directory, character) {
  return [
    `--user-data-dir=${fileURLToPath(new URL("profile/", directory))}`,
    "--new-window",
    papURL(character),
  ];
}

export async function openManualLogin(directory, channel) {
  if (process.platform !== "win32") throw new Error("Manual launcher supports Windows only");
  const relative = channel === "msedge" ? "Microsoft/Edge/Application/msedge.exe" : "Google/Chrome/Application/chrome.exe";
  const executable = [process.env.PROGRAMFILES, process.env["PROGRAMFILES(X86)"], process.env.LOCALAPPDATA]
    .filter(Boolean).map(root => path.join(root, relative)).find(existsSync);
  if (!executable) throw new Error("Browser is not installed");
  await mkdir(directory, { recursive: true, mode: 0o700 });
  return { executable, directory };
}

export async function launchManualLogin(directory, channel, character) {
  const browser = await openManualLogin(directory, channel);
  const child = spawn(browser.executable, manualLoginArgs(directory, character), {
    detached: true, stdio: "ignore", windowsHide: true,
  });
  await new Promise((resolve, reject) => { child.once("spawn", resolve); child.once("error", reject); });
  child.unref();
}

export async function saveJSON(directory, name, data) {
  const dest = new URL(name, directory);
  const temp = new URL(`${name}.tmp`, directory);
  await writeFile(temp, JSON.stringify(data, null, 2) + "\n", { mode: 0o600 });
  await rename(temp, dest);
}

export async function inspectPAP(page, character) {
  const response = await page.goto(papURL(character), {
    waitUntil: "domcontentloaded",
    timeout: 45000,
  });
  const state = classifyPage(page.url(), character, response?.status());
  if (state !== "inspectable") return { state };
  // Wait for visible page content; capture only the requested PAP page, not SSO forms.
  await page.locator("body").waitFor({ state: "visible" });
  const content = await page.evaluate(() => {
    const visible = (el) => el.getClientRects().length > 0;
    const cleanLink = (a) => {
      const url = new URL(a.href);
      // Keep only same-site paths for later verification of stable record identifiers.
      return url.origin === location.origin &&
        !/auth|logout|token/i.test(url.pathname)
        ? url.pathname
        : null;
    };
    return {
      title: document.title,
      text: document.body.innerText,
      tables: [...document.querySelectorAll("table")]
        .filter(visible)
        .map((table) => ({
          id: table.id,
          headers: [...table.querySelectorAll("thead th")]
            .filter(visible)
            .map((x) => x.innerText),
          rows: [...table.querySelectorAll("tbody tr")]
            .filter(visible)
            .map((row) => ({
              cells: [...row.querySelectorAll("td")]
                .filter(visible)
                .map((x) => x.innerText),
              links: [...row.querySelectorAll("a[href]")]
                .filter(visible)
                .map(cleanLink)
                .filter(Boolean),
            })),
        })),
    };
  });
  // Do not infer zero PAP or completeness from an empty/partially loaded table.
  if (classifyPage(page.url(), character) !== "inspectable")
    return { state: "login_required" };
  return {
    state: "awaiting_adapter",
    source: origin,
    character_id: character,
    observed_at: new Date().toISOString(),
    schema: "winterco.dom-inspection.v1",
    complete: false,
    importable: false,
    fingerprint: createHash("sha256")
      .update(JSON.stringify(content))
      .digest("hex"),
    content,
  };
}
