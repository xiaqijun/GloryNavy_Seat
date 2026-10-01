import type { Page } from "@playwright/test";
export async function mockCharacterSync(page: Page) {
  await page.route("**/api/v1/eve/sync/characters/*", (r) => {
    const id = r.request().url().split("/").at(-1)!;
    return r.fulfill({
      json: {
        data: {
          available: true,
          targets: ["profile", "authorization"].map((resource, index) => ({
            id: String(index + 1),
            character_id: id,
            name: "测试舰长",
            resource,
            state: "idle",
            freshness: "fresh",
            reason: "",
            last_attempt_at: new Date().toISOString(),
            last_success_at: new Date().toISOString(),
            content_updated_at: new Date().toISOString(),
            next_due_at: new Date(Date.now() + 600_000).toISOString(),
          })),
        },
      },
    });
  });
  await page.route("**/api/v1/eve/sync/characters/*/refresh", (r) =>
    r.fulfill({
      status: 202,
      json: {
        data: {
          results: [
            {
              target_id: "2",
              outcome: "deferred",
              next_due_at: new Date(Date.now() + 600_000).toISOString(),
            },
          ],
        },
      },
    }),
  );
}
