import { expect, type Page } from "@playwright/test";
import { mkdirSync } from "node:fs";

export async function captureDialogLayout(
  page: Page,
  title: string,
  name: string,
) {
  const dialog = page.getByRole("dialog", { name: title, exact: true });
  await expect(dialog).toBeVisible();
  const dir = "../.local/short-operation-dialogs/screenshots";
  mkdirSync(dir, { recursive: true });
  for (const width of [1440, 375, 320]) {
    await page.setViewportSize({ width, height: 720 });
    await expect
      .poll(() => dialog.evaluate((el) => el.scrollWidth <= el.clientWidth))
      .toBe(true);
    await expect(
      dialog.getByRole("button", { name: /^(取消|Cancel)$/ }),
    ).toBeInViewport();
    await page.screenshot({ path: `${dir}/${name}-${width}.png` });
  }
}
