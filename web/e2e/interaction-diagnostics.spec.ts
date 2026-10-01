import { test, expect, type Page } from "@playwright/test";

const key = "gnv:interaction-diagnostics";

async function setup(page: Page) {
  await page.route("**/api/v1/**", (route) =>
    route.fulfill({
      json: {
        data: new URL(route.request().url()).pathname.endsWith("/modules")
          ? [{ id: "system", version: "0.1.0", api_version: 1 }]
          : {
              name: "GloryNavy",
              version: "test",
              environment: "tranquility",
              database: "ready",
              schema_version: 1,
            },
      },
    }),
  );
  await page.goto("/workspace");
  await expect(
    page.getByRole("heading", { name: "系统状态", exact: true }),
  ).toBeVisible();
}

test("rapid navigation stays interactive; an injected orphan lock is recorded without force-unlocking", async ({
  page,
}) => {
  await setup(page);
  for (let round = 0; round < 10; round++) {
    await page
      .getByRole("navigation")
      .getByRole("link", { name: "系统状态" })
      .click();
    await expect(
      page.getByRole("heading", { name: "系统状态", exact: true }),
    ).toBeVisible();
    await page
      .getByRole("navigation")
      .getByRole("link", { name: "工作台" })
      .click();
  }
  expect(
    await page.evaluate((key) => sessionStorage.getItem(key), key),
  ).toBeNull();
  // Fault injection in an isolated browser context; this is not a reproduced app bug.
  await page.evaluate(() => {
    document.body.style.pointerEvents = "none";
  });
  await expect
    .poll(() =>
      page.evaluate(
        (key) => JSON.parse(sessionStorage.getItem(key) ?? "[]"),
        key,
      ),
    )
    .toContainEqual(
      expect.objectContaining({ kind: "orphan_pointer_lock", page: "/" }),
    );
  expect(
    await page
      .locator("body")
      .evaluate((el) => getComputedStyle(el).pointerEvents),
  ).toBe("none");
  await page.evaluate(() => {
    document.body.style.pointerEvents = "";
  });
  await page
    .getByRole("navigation")
    .getByRole("link", { name: "系统状态" })
    .click();
  await expect(
    page.getByRole("heading", { name: "系统状态", exact: true }),
  ).toBeVisible();
});

test("watchdog records a blocked main thread and evidence survives reload without URL queries", async ({
  page,
}) => {
  await setup(page);
  const ready = page.waitForRequest(
    (r) =>
      r.url().endsWith("/__debug/interaction") &&
      r.method() === "POST" &&
      r.postDataJSON()?.kind === "ready" &&
      r.postDataJSON()?.source === "main",
  );
  await page.goto("/system?private=test-only-do-not-record");
  const tab = (await ready).postDataJSON().tab;
  await expect(
    page.getByRole("heading", { name: "系统状态", exact: true }),
  ).toBeVisible();
  // Allow the worker to receive the first visible-page heartbeat.
  await page.waitForTimeout(1200);
  const recoveredAt = await page.evaluate(() => {
    const end = performance.now() + 6500;
    while (performance.now() < end) {
      /* Deliberate stall, isolated test only. */
    }
    return Date.now();
  });
  const reports = await (await page.request.get("/__debug/interaction")).json();
  const stalled = reports.entries.find(
    (e: { tab: string; kind: string; source: string }) =>
      e.tab === tab && e.kind === "main_thread_stall" && e.source === "worker",
  );
  expect(stalled).toBeTruthy();
  expect(Date.parse(stalled.at)).toBeLessThan(recoveredAt);
  await expect
    .poll(() => page.evaluate((key) => sessionStorage.getItem(key), key))
    .toContain("main_thread_stall");
  const evidence = await page.evaluate(
    (key) => sessionStorage.getItem(key),
    key,
  );
  expect(evidence).not.toContain("test-only-do-not-record");
  await page.reload();
  expect(
    await page.evaluate((key) => sessionStorage.getItem(key), key),
  ).toContain("main_thread_stall");
});
