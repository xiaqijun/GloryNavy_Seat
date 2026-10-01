import { test, expect, type Page } from "@playwright/test";
import { mkdirSync, readFileSync } from "node:fs";
const entity = { id: "0", name: "", category: "character" };
const contract = (id: string) => ({
  id,
  title: "",
  summary: `三钛合金 × ${id}`,
  trade_direction: "sell",
  type: "item_exchange",
  status: "outstanding",
  availability: "public",
  for_corporation: false,
  issuer: { ...entity, id: "123", name: "远航舰长" },
  assignee: entity,
  acceptor: entity,
  start: entity,
  end: entity,
  price: "123456789012345.6789",
  reward: null,
  collateral: null,
  buyout: null,
  volume: null,
  days_to_complete: null,
  date_issued: "2026-09-14T00:00:00Z",
  date_expired: "2026-09-20T00:00:00Z",
  date_accepted: "",
  date_completed: "",
  checked_at: "2026-09-14T04:00:00Z",
});
async function setup(page: Page) {
  let exporting = false;
  let fail = false;
  let pause = false;
  let release: (() => void) | undefined;
  const requests: URL[] = [];
  await page.route("https://images.evetech.net/**", (route) => route.abort());
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    const json = (data: unknown) => route.fulfill({ json: { data } });
    if (url.pathname === "/api/v1/modules")
      return json(
        ["system", "identity", "eve", "access"].map((id) => ({
          id,
          version: "0.1.0",
          api_version: 1,
        })),
      );
    if (url.pathname === "/api/v1/identity/session")
      return json({
        authenticated: true,
        session: {
          user_id: "test-only",
          character: { id: "123", name: "远航舰长" },
          csrf_token: "test-only",
          expires_at: "2099-01-01T00:00:00Z",
        },
      });
    if (url.pathname === "/api/v1/access/me")
      return json({
        administrator: false,
        can_manage: false,
        can_manage_sync: false,
        site_roles: [],
        characters: [],
      });
    if (url.pathname.endsWith("/owners"))
      return json({
        owners: [
          { kind: "character", id: "123", name: "远航舰长" },
          { kind: "corporation", id: "789", name: "Glory Navy" },
        ],
      });
    if (/\/contracts\/(character\/123|corporation\/789)$/.test(url.pathname)) {
      if (exporting) {
        requests.push(url);
        if (url.searchParams.get("before") === "2") {
          if (pause)
            await new Promise<void>((resolve) => {
              release = resolve;
            });
          if (fail)
            return route.fulfill({
              status: 404,
              json: { error: { message: "合同不存在或没有查看权限" } },
            });
          return json({ items: [contract("1")], next_cursor: "" });
        }
        return json({
          items: [contract("3"), contract("2")],
          next_cursor: "2",
        });
      }
      return json({ items: [contract("1")], next_cursor: "" });
    }
    return route.fulfill({
      status: 404,
      json: { error: { message: "Not found" } },
    });
  });
  return {
    start: (mode = "normal") => {
      exporting = true;
      fail = mode === "fail";
      pause = mode === "pause";
    },
    finish: () => {
      release?.();
    },
    requests,
  };
}

for (const kind of ["character", "corporation"])
  test(`${kind} exports all filtered pages with Chinese values`, async ({
    page,
  }) => {
    const controls = await setup(page);
    await page.goto(
      `/contracts?kind=${kind}&owner=${kind === "character" ? "123" : "789"}&before=2&q=补给&type=item_exchange&status=outstanding`,
    );
    const button = page.getByRole("button", { name: "导出合同（CSV）" });
    await expect(button).toBeEnabled();
    controls.start();
    const downloadPromise = page.waitForEvent("download");
    await button.click();
    const download = await downloadPromise;
    expect(download.suggestedFilename()).toMatch(
      /^(个人|军团)合同-\d+-[\d-]+\.csv$/,
    );
    const csv = readFileSync((await download.path())!, "utf8");
    expect(csv.charCodeAt(0)).toBe(0xfeff);
    expect(csv).toContain('"三钛合金 × 3"');
    expect(csv).toContain('"三钛合金 × 1"');
    expect(csv).toContain('"123456789012345.6789"');
    expect(csv).toContain('"公开"');
    expect(
      controls.requests.map((url) => url.searchParams.get("before")),
    ).toEqual([null, "2"]);
    for (const url of controls.requests) {
      expect(url.pathname).toContain(`/contracts/${kind}/`);
      expect(url.searchParams.get("q")).toBe("补给");
      expect(url.searchParams.get("type")).toBe("item_exchange");
      expect(url.searchParams.get("status")).toBe("outstanding");
    }
    await expect(
      page.getByRole("status").filter({ hasText: "已导出 3 条合同" }),
    ).toBeAttached();
  });

test("permission loss fails without downloading partial rows", async ({
  page,
}) => {
  const controls = await setup(page);
  const downloads: unknown[] = [];
  page.on("download", (event) => downloads.push(event));
  await page.goto("/contracts");
  await expect(
    page.getByRole("button", { name: "导出合同（CSV）" }),
  ).toBeEnabled();
  controls.start("fail");
  await page.getByRole("button", { name: "导出合同（CSV）" }).click();
  await expect(page.getByRole("alert")).toHaveText("合同不存在或没有查看权限");
  expect(downloads).toHaveLength(0);
});

for (const cancel of ["button", "filter"])
  test(`cancellation by ${cancel} prevents a delayed download`, async ({
    page,
  }) => {
    const controls = await setup(page);
    const downloads: unknown[] = [];
    page.on("download", (event) => downloads.push(event));
    await page.goto("/contracts");
    await expect(
      page.getByRole("button", { name: "导出合同（CSV）" }),
    ).toBeEnabled();
    controls.start("pause");
    await page.getByRole("button", { name: "导出合同（CSV）" }).click();
    await expect.poll(() => controls.requests.length).toBe(2);
    if (cancel === "button")
      await page.getByRole("button", { name: /取消导出/ }).click();
    else {
      await page.getByRole("combobox", { name: "合同状态" }).click();
      await page.getByRole("option", { name: "已结束", exact: true }).click();
    }
    // Wait for the route transition to commit before releasing the old scope's response.
    await expect(page.getByRole("button", { name: /取消导出/ })).toHaveCount(0);
    controls.finish();
    await expect(
      page.getByRole("button", { name: "导出合同（CSV）" }),
    ).toBeEnabled();
    await expect(page.getByRole("button", { name: /取消导出/ })).toHaveCount(0);
    expect(downloads).toHaveLength(0);
  });

test("export toolbar fits narrow screens", async ({ page }) => {
  await setup(page);
  for (const width of [1440, 375, 320]) {
    await page.setViewportSize({ width, height: 900 });
    await page.goto("/contracts");
    const button = page.getByRole("button", { name: "导出合同（CSV）" });
    await expect(button).toBeEnabled();
    const box = await button.boundingBox();
    expect(box!.width).toBeGreaterThanOrEqual(44);
    expect(box!.height).toBeGreaterThanOrEqual(44);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    mkdirSync("../docs/ui/reviews/contracts-export", { recursive: true });
    await page.screenshot({
      path: `../docs/ui/reviews/contracts-export/toolbar-${test.info().project.name}-${width}.png`,
      fullPage: true,
    });
  }
});
