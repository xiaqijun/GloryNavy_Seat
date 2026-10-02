import { defineConfig, devices } from "@playwright/test";
export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  // GitHub's runner exposes two workers by default.  The suite has 476
  // generated project tests, so running with a small fixed pool keeps the
  // run within the job budget without allowing unbounded browser pressure.
  workers: process.env.CI ? 4 : undefined,
  globalTimeout: process.env.CI ? 30 * 60 * 1000 : undefined,
  use: {
    baseURL: "http://127.0.0.1:5173",
    trace: "retain-on-failure",
    launchOptions: {
      executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE,
      args: process.env.PLAYWRIGHT_DISABLE_GPU === "1" ? ["--disable-gpu"] : [],
    },
  },
  projects: [
    { name: "desktop", use: { ...devices["Desktop Chrome"] } },
    { name: "mobile", use: { ...devices["Pixel 7"] } },
  ],
  webServer: [
    {
      command: "node ../scripts/tasks.mjs api",
      url: "http://127.0.0.1:8080/health/ready",
      reuseExistingServer: !process.env.CI,
      timeout: 120000,
    },
    {
      command: "npm run dev",
      url: "http://127.0.0.1:5173",
      reuseExistingServer: !process.env.CI,
    },
  ],
});
