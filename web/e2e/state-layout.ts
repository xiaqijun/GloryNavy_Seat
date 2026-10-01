import { test, expect, type Page } from "@playwright/test";

export async function assertLayout(page: Page) {
  await expect
    .poll(() =>
      page.evaluate(() => ({
        page: document.documentElement.scrollWidth > innerWidth + 1,
        dialogs: [...document.querySelectorAll<HTMLElement>('[role="dialog"]')]
          .filter((el) => el.getClientRects().length)
          .some((el) => {
            const r = el.getBoundingClientRect();
            return (
              r.left < 0 ||
              r.right > innerWidth + 1 ||
              r.top < 0 ||
              r.bottom > innerHeight + 1 ||
              el.scrollWidth > el.clientWidth + 1
            );
          }),
      })),
    )
    .toEqual({ page: false, dialogs: false });
}

// Each caller reuses its business fixture; these are read failures, never real
// submissions. Keep loading and empty separate so missing data cannot pass as ready.
export function stateLayouts(options: {
  name: string;
  setup: (page: Page) => Promise<unknown>;
  path: string;
  endpoint: string;
  empty?: unknown;
  emptyText?: RegExp;
  errorText?: RegExp;
}) {
  for (const locale of ["zh-CN", "en"]) {
    test(`state layout ${options.name} ${locale}: loading, long error, recovery${options.empty === undefined ? "" : ", empty"}`, async ({
      page,
    }, info) => {
      test.setTimeout(60000);
      await options.setup(page);
      await page.addInitScript(
        (locale) => localStorage.setItem("glorynavy.locale", locale),
        locale,
      );
      let release!: () => void;
      const gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      const resource = options.endpoint.replaceAll("*", "").split("?")[0];
      const requested = page.waitForRequest(
        (request) => request.url().includes(resource),
        { timeout: 15000 },
      );
      const message =
        "Service temporarily unavailable / 暂时无法读取：" +
        "LongUnbrokenReference".repeat(12);
      let failed = true;
      await page.route(options.endpoint, async (route) => {
        if (!failed) return route.fallback();
        await gate;
        await route.fulfill({ status: 503, json: { error: { message } } });
      });
      await page.goto(options.path);
      await requested;
      try {
        for (const width of [1440, 375, 320]) {
          await page.setViewportSize({ width, height: 800 });
          await expect(page.locator("main")).toBeVisible();
          await assertLayout(page);
        }
        await page.screenshot({
          path: info.outputPath("loading-320.png"),
          fullPage: true,
        });
      } finally {
        release();
      }
      const error = page
        .getByRole("alert")
        .filter({ hasText: options.errorText ?? message })
        .first();
      await expect(error).toBeVisible({ timeout: 15000 });
      for (const width of [1440, 375, 320]) {
        await page.setViewportSize({ width, height: 800 });
        await assertLayout(page);
      }
      await page.screenshot({
        path: info.outputPath("error-320.png"),
        fullPage: true,
      });
      failed = false;
      const recovered = page.waitForResponse(
        (response) =>
          response
            .url()
            .includes(options.endpoint.replaceAll("*", "").split("?")[0]) &&
          response.status() === 200,
      );
      await page.reload();
      await recovered;
      await expect(
        page
          .getByRole("alert")
          .filter({ hasText: options.errorText ?? message }),
      ).toHaveCount(0);
      await expect(page.locator("main h1")).toBeVisible();
      if (options.empty !== undefined) {
        await page.route(options.endpoint, (route) =>
          route.fulfill({ json: { data: options.empty } }),
        );
        await page.reload();
        await expect(
          page.locator("main").getByText(options.emptyText!).first(),
        ).toBeVisible();
        for (const width of [1440, 375, 320]) {
          await page.setViewportSize({ width, height: 800 });
          await assertLayout(page);
        }
        await page.screenshot({
          path: info.outputPath("empty-320.png"),
          fullPage: true,
        });
      }
    });
  }
}
