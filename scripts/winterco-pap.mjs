import { parseArgs } from "node:util";
import {
  openBrowser,
  launchManualLogin,
  papURL,
  inspectPAP,
  saveJSON,
} from "./winterco/browser.mjs";

const { values } = parseArgs({
  options: {
    mode: { type: "string", default: "inspect" },
    character: { type: "string" },
    browser: { type: "string", default: "msedge" },
    help: { type: "boolean" },
  },
});
if (values.help) {
  console.log(
    "npm run pap:browser -- --mode login|inspect --character <EVE character ID> [--browser msedge|chrome]\nLogin is interactive. Inspection saves a local, non-importable PAP page snapshot; no PAP or coins are issued.",
  );
  process.exit(0);
}
if (
  !["login", "inspect"].includes(values.mode) ||
  !["msedge", "chrome"].includes(values.browser)
) {
  throw new Error("Unsupported mode or browser");
}
const url = papURL(values.character || "");
const directory = new URL(`../.local/winterco-browser/${values.browser}/`, import.meta.url);
let context;
let stopping = false;
async function close() {
  stopping = true;
  await context?.close();
}
process.once("SIGINT", () => void close());
process.once("SIGTERM", () => void close());
try {
  if (values.mode === "login") {
    await launchManualLogin(directory, values.browser, values.character);
    console.log("已打开普通浏览器专用配置。请手动登录、打开 PAP 页面，然后关闭该专用窗口，再运行 inspect。\nNormal browser opened with a dedicated profile. Log in, open the PAP page, then close this window before running inspect.");
    process.exitCode = 0;
  } else {
  context = await openBrowser(
    directory,
    values.browser,
    values.mode !== "login",
  );
  const page = context.pages()[0] || (await context.newPage());
    const snapshot = await inspectPAP(page, values.character);
    if (snapshot.state === "awaiting_adapter") {
      await saveJSON(
        directory,
        `inspection-${values.character}.json`,
        snapshot,
      );
    }
    await saveJSON(directory, "status.json", {
      state: snapshot.state,
      character_id: values.character,
      checked_at: new Date().toISOString(),
      imported: false,
    });
    console.log(JSON.stringify({ state: snapshot.state, imported: false }));
    if (snapshot.state !== "awaiting_adapter") process.exitCode = 2;
  }
} catch {
  if (!stopping) {
    // Avoid printing browser stack traces, cookies, login URLs or OAuth query parameters.
    console.error(
      "浏览器读取失败；旧快照保留。请检查网络，并先关闭占用专用配置的浏览器。",
    );
    console.error(
      "Browser read failed; previous snapshot retained. Check network and close the dedicated profile before retrying.",
    );
    process.exitCode = 1;
  }
} finally {
  await context?.close();
}
