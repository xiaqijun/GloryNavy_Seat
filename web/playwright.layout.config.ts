import { defineConfig, devices } from "@playwright/test";
import base from "./playwright.config";

// A release-build UI check. Diagnostic worker injection is DEV-only and belongs
// to the normal dev-server suite, not this preview/browser matrix.
export default defineConfig({
  ...base,
  testIgnore: "**/interaction-diagnostics.spec.ts",
  outputDir:
    process.env.LAYOUT_RESULT_DIR || "../.local/layout-regression/results",
  reporter: [
    ["line"],
    [
      "json",
      {
        outputFile:
          process.env.LAYOUT_REPORT_FILE ||
          "../.local/layout-regression/results.json",
      },
    ],
  ],
  projects: ["chrome", "msedge"].flatMap((channel) => [
    {
      name: `${channel}-desktop`,
      use: { ...devices["Desktop Chrome"], channel, launchOptions: {} },
    },
    {
      name: `${channel}-mobile`,
      use: { ...devices["Pixel 7"], channel, launchOptions: {} },
    },
  ]),
});
