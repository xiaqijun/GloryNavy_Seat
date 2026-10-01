import { expect, type Page } from "@playwright/test";
import { mkdirSync } from "node:fs";

export async function englishLayout(
  page: Page,
  title: string,
  artifact: string,
) {
  await page.getByRole("button", { name: "切换为英文", exact: true }).click();
  await expect(page.locator("html")).toHaveAttribute("lang", "en");
  await expect(
    page.getByRole("heading", { name: title, exact: true }),
  ).toBeVisible();
  for (const width of [1440, 375, 320]) {
    await page.setViewportSize({ width, height: 900 });
    await expect
      .poll(() =>
        page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
      )
      .toBe(true);
    mkdirSync("../.local/i18n/screenshots", { recursive: true });
    await page.screenshot({
      path: `../.local/i18n/screenshots/${artifact}-${width}.png`,
      fullPage: true,
    });
  }
}
