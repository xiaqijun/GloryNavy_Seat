import { test, expect, type Page } from "@playwright/test";
import { stateLayouts } from "./state-layout";
import { mkdirSync } from "node:fs";
import { fileURLToPath } from "node:url";
async function setup(page: Page, allowed = true) {
  await page.route("https://images.evetech.net/**", (r) => r.abort());
  await page.route("**/api/v1/modules", (r) =>
    r.fulfill({
      json: {
        data: ["system", "identity", "eve", "access"].map((id) => ({
          id,
          version: "0.1.0",
          api_version: 1,
        })),
      },
    }),
  );
  await page.route("**/api/v1/identity/session", (r) =>
    r.fulfill({
      json: {
        data: {
          authenticated: true,
          session: {
            user_id: "test-user",
            character: { id: "123", name: "同步舰长" },
            csrf_token: "test-csrf",
            expires_at: "2099-01-01T00:00:00Z",
          },
        },
      },
    }),
  );
  await page.route("**/api/v1/access/me", (r) =>
    r.fulfill({
      json: {
        data: {
          administrator: false,
          can_manage: !allowed,
          can_manage_sync: allowed,
          site_roles: [],
          characters: [],
        },
      },
    }),
  );
  let retried = 0;
  await page.route("**/api/v1/eve/sync/targets?**", (r) => {
    const filter = new URL(r.request().url()).searchParams.get("state");
    const targets = [
      {
        id: "1",
        character_id: "123",
        name: "荣耀远航后勤与工业支援舰长",
        resource: "profile",
        state: retried ? "queued" : "failed",
        reason: retried ? "" : "network_error",
        freshness: "stale",
        last_attempt_at: "2026-09-14T02:00:00Z",
        last_success_at: "2026-09-14T01:00:00Z",
        content_updated_at: "2026-09-14T01:00:00Z",
        next_due_at: "2026-09-14T03:00:00Z",
      },
      {
        id: "2",
        character_id: "456",
        name: "侦察舰长",
        resource: "authorization",
        state: "blocked",
        reason: "reauthorize",
        freshness: "never",
        last_attempt_at: "2026-09-14T02:00:00Z",
        last_success_at: null,
        content_updated_at: null,
        next_due_at: "2026-09-14T03:00:00Z",
      },
    ];
    return r.fulfill({
      json: {
        data: {
          available: true,
          targets: filter ? targets.filter((t) => t.state === filter) : targets,
          next_cursor: "",
        },
      },
    });
  });
  await page.route("**/api/v1/eve/sync/targets/1/retry", (r) => {
    expect(r.request().headers()["x-csrf-token"]).toBe("test-csrf");
    retried++;
    return r.fulfill({
      status: 202,
      json: {
        data: {
          results: [
            {
              target_id: "1",
              outcome: "queued",
              next_due_at: new Date().toISOString(),
            },
          ],
        },
      },
    });
  });
  await page.route("**/api/v1/eve/sync/targets/*/runs", (r) =>
    r.fulfill({
      json: {
        data: {
          runs: [
            {
              id: "1",
              started_at: "2026-09-14T02:00:00Z",
              finished_at: "2026-09-14T02:00:05Z",
              outcome: "failed",
              reason: "network_error",
              http_status: 0,
            },
          ],
        },
      },
    }),
  );
  return () => retried;
}

const observedToken = {
  character_id: "123",
  generation: "2",
  name: "荣耀远航后勤与工业支援舰长",
  state: "valid",
  scopes: [
    "esi-characters.read_corporation_roles.v1",
    "esi-contracts.read_corporation_contracts.v1",
  ],
  observed_since: "2026-09-14T14:00:00Z",
  access_expires_at: "2026-09-14T14:50:00Z",
  last_used_at: "2026-09-14T14:30:00Z",
  reuse_count: 124,
  last_refresh_attempt_at: "2026-09-14T14:30:00Z",
  last_refresh_success_at: "2026-09-14T14:30:00Z",
  last_refresh_reason: "",
  refresh_successes: 3,
  refresh_failures: 0,
  consecutive_failures: 0,
  last_request_at: "2026-09-14T14:30:00Z",
  last_request_status: 200,
  last_request_reason: "",
  network_requests: 102,
  cache_hits: 22,
  rate_limit_waits: 8,
  request_failures: 0,
};
test("mixed sync resources accept training queues without failing the list", async ({
  page,
}) => {
  await setup(page);
  const resources = [
    "profile",
    "authorization",
    "character_contracts",
    "corporation_contracts",
    "fittings",
    "skills",
    "skillqueue",
  ];
  await page.route("**/api/v1/eve/sync/targets?**", (r) =>
    r.fulfill({
      json: {
        data: {
          available: true,
          next_cursor: "",
          targets: resources.map((resource, i) => ({
            id: String(i + 1),
            character_id: "123",
            name: "同步舰长",
            resource,
            state: "idle",
            reason: "",
            freshness: "fresh",
            last_attempt_at: "2026-09-15T09:38:00Z",
            last_success_at: "2026-09-15T09:38:00Z",
            content_updated_at: "2026-09-15T09:38:00Z",
            next_due_at: "2099-01-01T00:00:00Z",
          })),
        },
      },
    }),
  );
  await page.goto("/sync");
  await expect(page.getByText("训练队列", { exact: true })).toBeVisible();
  await expect(page.getByText("舰船配置", { exact: true })).toBeVisible();
  await expect(page.getByText("同步列表读取失败")).toHaveCount(0);
  await expect(page.getByText("已更新", { exact: true })).toHaveCount(7);
});
test("token observation remains readable with records and keyboard controls", async ({
  page,
}, info) => {
  await setup(page);
  await page.route("**/api/v1/eve/sync/tokens?**", (r) => {
    const filter = new URL(r.request().url()).searchParams.get("state");
    const tokens = [
      observedToken,
      {
        ...observedToken,
        character_id: "456",
        name: "侦察舰长",
        state: "refresh_failed",
        last_refresh_reason: "sso_unavailable",
        refresh_failures: 1,
        consecutive_failures: 1,
      },
      {
        ...observedToken,
        character_id: "789",
        name: "后勤舰长",
        state: "unknown",
        access_expires_at: null,
        observed_since: null,
        last_refresh_success_at: null,
      },
    ];
    return r.fulfill({
      json: {
        data: {
          tokens: filter ? tokens.filter((t) => t.state === filter) : tokens,
          next_cursor: "",
          observed_at: "2026-09-14T14:31:00Z",
        },
      },
    });
  });
  await page.route("**/api/v1/eve/sync/tokens/123/events", (r) =>
    r.fulfill({
      json: {
        data: {
          events: [
            {
              id: "1",
              generation: "2",
              occurred_at: "2026-09-14T14:30:00Z",
              outcome: "refresh_success",
              reason: "",
              duration_ms: 412,
            },
          ],
        },
      },
    }),
  );
  await page.goto("/sync?view=tokens");
  await expect(
    page.getByRole("tab", { name: "登录令牌", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  await expect(page.getByText("有效期内", { exact: true })).toBeVisible();
  await expect(page.getByText("刷新异常", { exact: true })).toBeVisible();
  await expect(page.getByText("尚未观测", { exact: true })).toBeVisible();
  await page
    .getByRole("button", { name: "查看荣耀远航后勤与工业支援舰长的令牌记录" })
    .click();
  await expect(page.getByText("刷新成功", { exact: true })).toBeVisible();
  const screenshots = fileURLToPath(
    new URL("../../docs/ui/reviews/tokens/", import.meta.url),
  );
  mkdirSync(screenshots, { recursive: true });
  for (const width of info.project.name === "desktop"
    ? [1440, 375, 320]
    : [375]) {
    await page.setViewportSize({ width, height: 1000 });
    await expect(
      page.getByRole("table", { name: "ESI 令牌观测" }),
    ).toBeVisible();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: `${screenshots}/${info.project.name}-${width}.png`,
      fullPage: true,
    });
  }
  const select = page.getByRole("combobox", { name: "令牌状态" });
  await select.focus();
  await page.keyboard.press("Enter");
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("Escape");
  await expect(select).toBeFocused();
  await select.click();
  await page.getByRole("option", { name: "刷新异常", exact: true }).click();
  await expect(page.getByText(observedToken.name, { exact: true })).toHaveCount(
    0,
  );
  await page.getByRole("tab", { name: "登录令牌", exact: true }).focus();
  await page.keyboard.press("Home");
  await expect(
    page.getByRole("tab", { name: "同步任务", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
});

test("token observation handles unavailable and empty data", async ({
  page,
}) => {
  await setup(page);
  let fail = true;
  await page.route("**/api/v1/eve/sync/tokens?**", (r) =>
    fail
      ? r.fulfill({ status: 503, json: { error: { message: "暂不可用" } } })
      : r.fulfill({
          json: {
            data: {
              tokens: [],
              next_cursor: "",
              observed_at: "2026-09-14T14:31:00Z",
            },
          },
        }),
  );
  await page.goto("/sync?view=tokens");
  await expect(
    page.getByText("令牌状态读取失败", { exact: true }),
  ).toBeVisible();
  fail = false;
  await page.getByRole("button", { name: "重试令牌列表" }).click();
  await expect(page.getByText("没有匹配的角色", { exact: true })).toBeVisible();
});
test("sync permissions stay independent from permission administration", async ({
  page,
}) => {
  await setup(page, false);
  await page.goto("/sync");
  await expect(
    page.getByText("没有同步管理权限", { exact: true }),
  ).toBeVisible();
  await expect(
    page
      .getByRole("navigation")
      .getByRole("link", { name: "ESI 同步", exact: true }),
  ).toHaveCount(0);
});
test("personal and corporation contract progress remains explicit", async ({
  page,
}, info) => {
  await setup(page);
  await page.route("**/api/v1/eve/sync/targets?**", (r) =>
    r.fulfill({
      json: {
        data: {
          available: true,
          next_cursor: "",
          targets: [
            {
              id: "10",
              character_id: "123",
              name: "合同舰长",
              resource: "character_contracts",
              state: "idle",
              reason: "",
              freshness: "fresh",
              last_attempt_at: null,
              last_success_at: "2026-09-14T02:00:00Z",
              content_updated_at: null,
              next_due_at: "2026-09-14T03:00:00Z",
              pending_details: 3,
              failed_details: 0,
            },
            {
              id: "11",
              character_id: "123",
              name: "合同舰长",
              resource: "corporation_contracts",
              state: "deferred",
              reason: "shared_source",
              freshness: "never",
              last_attempt_at: null,
              last_success_at: null,
              content_updated_at: null,
              next_due_at: "2026-09-14T03:00:00Z",
              pending_details: 0,
              failed_details: 0,
            },
          ],
        },
      },
    }),
  );
  await page.goto("/sync");
  await expect(page.getByText("个人合同", { exact: true })).toBeVisible();
  await expect(page.getByText("军团合同", { exact: true })).toBeVisible();
  await expect(page.getByText("列表已更新", { exact: true })).toBeVisible();
  await expect(
    page.getByText("明细 3 项待同步", { exact: true }),
  ).toBeVisible();
  await expect(page.getByText("共享同步", { exact: true })).toBeVisible();
  await page.setViewportSize({ width: 320, height: 812 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  mkdirSync("../docs/ui/reviews/contracts", { recursive: true });
  await page.screenshot({
    path: `../docs/ui/reviews/contracts/${info.project.name}-320.png`,
    fullPage: true,
  });
});
test("sync status, records and retry remain usable at narrow widths", async ({
  page,
}, info) => {
  const attempts = await setup(page);
  await page.goto("/sync");
  await expect(
    page.getByRole("heading", { name: "ESI 同步", exact: true }),
  ).toBeVisible();
  await expect(
    page
      .getByRole("navigation")
      .getByRole("link", { name: "权限管理", exact: true }),
  ).toHaveCount(0);
  await expect(page.getByText("需要重新授权", { exact: true })).toBeVisible();
  mkdirSync("../docs/ui/reviews/sync", { recursive: true });
  for (const width of info.project.name === "desktop"
    ? [1440, 375, 320]
    : [375]) {
    await page.setViewportSize({ width, height: 900 });
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: `../docs/ui/reviews/sync/${info.project.name}-${width}.png`,
      fullPage: true,
    });
  }
  await page.getByRole("button", { name: "查看基础资料运行记录" }).click();
  await expect(
    page.getByRole("button", { name: "收起基础资料运行记录" }),
  ).toHaveAttribute("aria-expanded", "true");
  await page
    .getByRole("button", { name: "重试荣耀远航后勤与工业支援舰长的基础资料" })
    .click();
  await expect(page.getByText("已加入同步队列", { exact: true })).toBeVisible();
  await expect(
    page
      .getByRole("table", { name: "ESI 同步任务" })
      .getByText("已排队", { exact: true }),
  ).toBeVisible();
  expect(attempts()).toBe(1);
  await page.getByLabel("同步状态", { exact: true }).selectOption("blocked");
  await expect(
    page.getByText("荣耀远航后勤与工业支援舰长", { exact: true }),
  ).toHaveCount(0);
});

const rateBucket = {
  id: "1",
  group: "corp-contract",
  policy_source: "response",
  character_id: "123",
  name: "荣耀远航后勤与工业支援舰长",
  observed_since: "2026-09-14T12:00:00Z",
  header_at: "2026-09-14T12:01:00Z",
  capacity: 600,
  remaining: 428,
  window_seconds: 900,
  retry_at: null,
  local_remaining: 95,
  local_recovery_at: "2026-09-14T12:16:00Z",
  local_blocked_until: null,
  egress_blocked_until: null,
  used_tokens: 172,
  network_requests: 98,
  unmeasured_requests: 3,
};
const rateRoute = {
  route: "GET /corporations/{id}/contracts/{id}/items/",
  network_requests: 86,
  cache_hits: 112,
  local_waits: 6,
  upstream_limits: 0,
  used_tokens: 172,
  measured_responses: 86,
  unmeasured_requests: 0,
  last_status: 200,
  last_used: 2,
  last_response_at: "2026-09-14T12:01:00Z",
};
test("rate buckets show endpoint consumption and responsive snapshots", async ({
  page,
}, info) => {
  await setup(page);
  await page.route("**/api/v1/eve/sync/rate-limits?**", (r) =>
    r.fulfill({
      json: {
        data: {
          buckets:
            new URL(r.request().url()).searchParams.get("search") === "none"
              ? []
              : [
                  rateBucket,
                  {
                    ...rateBucket,
                    id: "2",
                    group: "char-contract",
                    name: "侦察舰长",
                    character_id: "456",
                    local_remaining: 0,
                  },
                  {
                    ...rateBucket,
                    id: "3",
                    group: "status",
                    policy_source: "openapi",
                    name: "",
                    character_id: "",
                    capacity: 600,
                    remaining: null,
                    header_at: null,
                    window_seconds: 900,
                    local_remaining: null,
                    local_recovery_at: null,
                  },
                ],
          next_cursor: "",
          observed_at: "2026-09-14T12:02:00Z",
        },
      },
    }),
  );
  await page.route("**/api/v1/eve/sync/rate-limits/1/routes?**", (r) =>
    r.fulfill({
      json: {
        data: {
          routes: [
            rateRoute,
            {
              ...rateRoute,
              route: "GET /corporations/{id}/contracts/",
              used_tokens: 0,
              measured_responses: 0,
              unmeasured_requests: 3,
              last_used: null,
            },
          ],
          next_cursor: "",
        },
      },
    }),
  );
  await page.goto("/sync?view=rate-limits");
  await expect(
    page.getByRole("tab", { name: "令牌桶", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  await expect(page.getByText("本地预算等待", { exact: true })).toBeVisible();
  await page
    .getByRole("button", {
      name: "查看corp-contract接口消耗",
      exact: true,
    })
    .click();
  await expect(
    page.getByRole("img", { name: /^本页接口已计量消耗/ }).locator("svg"),
  ).toBeVisible();
  await expect(page.getByText("未识别分组", { exact: true })).toHaveCount(0);
  await expect(
    page.getByRole("img", { name: /合同列表\s+未计量：未计量/ }),
  ).toBeVisible();
  const screenshots = fileURLToPath(
    new URL("../../docs/ui/reviews/rate-limits/", import.meta.url),
  );
  mkdirSync(screenshots, { recursive: true });
  for (const width of info.project.name === "desktop"
    ? [1440, 375, 320]
    : [375]) {
    await page.setViewportSize({ width, height: 960 });
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: `${screenshots}/${info.project.name}-${width}.png`,
      fullPage: true,
    });
  }
  await page.getByRole("button", { name: "查看接口明细", exact: true }).focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("table", { name: "接口令牌消耗" })).toBeVisible();
  await expect(page.getByText(rateRoute.route, { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "查看消耗图表", exact: true }).click();
  await page.emulateMedia({ reducedMotion: "reduce" });
  await expect(
    page.getByRole("img", { name: /^本页接口已计量消耗/ }).locator("svg"),
  ).toBeVisible();
  await page
    .getByRole("button", {
      name: "收起corp-contract接口消耗",
      exact: true,
    })
    .focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("table", { name: "接口令牌消耗" })).toHaveCount(
    0,
  );
  await page.getByRole("textbox", { name: "搜索令牌桶或角色" }).fill("none");
  await page.getByRole("button", { name: "搜索令牌桶", exact: true }).click();
  await expect(
    page.getByText("暂无匹配的令牌桶", { exact: true }),
  ).toBeVisible();
});
test("rate bucket failures remain recoverable", async ({ page }) => {
  await setup(page);
  let fail = true;
  await page.route("**/api/v1/eve/sync/rate-limits?**", (r) =>
    fail
      ? r.fulfill({ status: 503, json: { error: { message: "暂不可用" } } })
      : r.fulfill({
          json: {
            data: {
              buckets: [],
              next_cursor: "",
              observed_at: "2026-09-14T12:02:00Z",
            },
          },
        }),
  );
  await page.goto("/sync?view=rate-limits");
  await expect(page.getByText("令牌桶读取失败", { exact: true })).toBeVisible();
  fail = false;
  await page.getByRole("button", { name: "重试令牌桶", exact: true }).click();
  await expect(
    page.getByText("暂无匹配的令牌桶", { exact: true }),
  ).toBeVisible();
});

async function layoutSetup(page: Page) {
  await setup(page);
  await page.route("**/api/v1/eve/sync/tokens?**", r => r.fulfill({ json: { data: { tokens: [observedToken], next_cursor: "", observed_at: "2026-09-20T00:00:00Z" } } }));
  await page.route("**/api/v1/eve/sync/rate-limits?**", r => r.fulfill({ json: { data: { buckets: [rateBucket], next_cursor: "", observed_at: "2026-09-20T00:00:00Z" } } }));
}
stateLayouts({ name: "sync", errorText: /同步列表读取失败|Unable to load sync list/, setup: layoutSetup, path: "/sync", endpoint: "**/api/v1/eve/sync/targets?**", empty: { targets: [], available: true, next_cursor: "" }, emptyText: /暂无匹配的同步任务|No matching sync tasks/ });
stateLayouts({ name: "tokens", errorText: /令牌状态读取失败|Unable to load token status/, setup: layoutSetup, path: "/sync?view=tokens", endpoint: "**/api/v1/eve/sync/tokens?**", empty: { tokens: [], next_cursor: "", observed_at: "2026-09-20T00:00:00Z" }, emptyText: /没有匹配的角色|No matching characters/ });
stateLayouts({ name: "rate-limits", errorText: /令牌桶读取失败|Unable to load token buckets/, setup: layoutSetup, path: "/sync?view=rate-limits", endpoint: "**/api/v1/eve/sync/rate-limits?**", empty: { buckets: [], next_cursor: "", observed_at: "2026-09-20T00:00:00Z" }, emptyText: /暂无匹配的令牌桶|No matching token buckets/ });
