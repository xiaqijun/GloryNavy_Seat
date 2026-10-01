import { expect, type Page } from "@playwright/test";

export async function openNavigation(page: Page, group?: string) {
  await expect(page.locator("main h1")).toBeVisible();
  const toggle = page.getByRole("button", { name: "菜单", exact: true });
  if (await toggle.isVisible()) {
    if ((await toggle.getAttribute("aria-expanded")) !== "true")
      await toggle.click();
  }
  const nav = page.getByRole("navigation", { name: "主导航" });
  if (group) {
    const trigger = nav.getByRole("button", { name: group, exact: true });
    if ((await trigger.getAttribute("aria-expanded")) !== "true") {
      // Permissions and route data may finish while the click becomes actionable.
      await nav
        .locator("button[aria-expanded='false']")
        .filter({ hasText: group })
        .click({ trial: true });
      if ((await trigger.getAttribute("aria-expanded")) !== "true")
        await trigger.click();
    }
  }
  await expect(nav).toBeVisible();
  return nav;
}

export async function navigate(page: Page, name: string, group?: string) {
  const nav = await openNavigation(page, group);
  await nav.getByRole("link", { name, exact: true }).click();
}
