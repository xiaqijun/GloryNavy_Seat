import { defineConfig } from "@playwright/test";
import base from "./playwright.config";

// A diagnostic comparison, not a replica of the user's existing Edge profile.
// Other Playwright defaults still apply; no user's browser settings are changed.
export default defineConfig(
  { ...base, projects: [] },
  {
    workers: 1,
    use: {
      headless: false,
      trace: "off",
      launchOptions: {
        executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE,
        args:
          process.env.PLAYWRIGHT_DISABLE_GPU === "1" ? ["--disable-gpu"] : [],
        ignoreDefaultArgs: [
          "--disable-hang-monitor",
          "--disable-ipc-flooding-protection",
          "--disable-background-timer-throttling",
          "--disable-backgrounding-occluded-windows",
          "--disable-renderer-backgrounding",
        ],
      },
    },
    projects: [
      { name: "edge-repro", use: { viewport: { width: 1280, height: 900 } } },
    ],
  },
);
