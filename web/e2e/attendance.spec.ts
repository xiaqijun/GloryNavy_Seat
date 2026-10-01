import { stateLayouts } from "./state-layout";
import { captureDialogLayout } from "./dialog-layout";
import { test, expect, type Page } from "@playwright/test";
import { englishLayout } from "./language-layout";
import { mkdirSync } from "node:fs";
import { fileURLToPath } from "node:url";
async function setup(page: Page, manage = true, pap = false) {
  await page.route("https://images.evetech.net/**", (r) => r.abort());
  await page.route("**/api/v1/modules", (r) =>
    r.fulfill({
      json: {
        data: ["system", "identity", "eve", "access", "attendance"].map(
          (id) => ({ id, api_version: 1, version: "0.1.0" }),
        ),
      },
    }),
  );
  await page.route("**/api/v1/identity/session", (r) =>
    r.fulfill({
      json: {
        data: {
          authenticated: true,
          session: {
            user_id: "user",
            character: { id: "123", name: "Hajimi1" },
            csrf_token: "csrf",
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
          administrator: manage,
          can_manage: false,
          can_manage_sync: false,
          site_roles: [],
          characters: [],
        },
      },
    }),
  );
  let version = 1,
    captured = pap;
  let papIssued = false,
    papPoints = 1;
  const papHistory: {
    id: string;
    character_id: string;
    name: string;
    delta: number;
    balance: number;
    reason: string;
    created_at: string;
  }[] = [];
  const event = () => ({
    id: "1",
    corporation_id: "10",
    title: "荣耀远航 · 周末联合舰队",
    starts_at: "2026-09-15T11:30:00Z",
    ends_at: pap ? "2026-09-15T13:45:00Z" : null,
    state: pap ? "closed" : "open",
    pap_points: papPoints,
    pap_issued: papIssued,
    version: String(version),
    participants: captured ? 2 : 0,
    can_manage: manage,
    can_convert: manage,
  });
  await page.route("**/api/v1/attendance/**", async (r) => {
    const url = new URL(r.request().url());

    if (url.pathname.endsWith("/events/1/pap")) {
      if (r.request().method() === "POST") {
        expect(manage).toBe(true);
        expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
        const c = r.request().postDataJSON();
        expect(c.version).toBe(String(version));
        expect(c.reason.length).toBeGreaterThan(0);
        const balance = c.revoke ? 0 : c.points;
        const previous = papIssued ? papPoints : 0;
        for (let i = 0; i < 3; i++)
          papHistory.push({
            id: String(papHistory.length + 1),
            character_id: String(123 + i),
            name: ["Hajimi1", "荣耀远航后勤与工业支援舰长", "Nuter Zero"][i],
            delta: balance - previous,
            balance,
            reason: c.reason,
            created_at: "2026-09-15T12:30:00Z",
          });
        papIssued = !c.revoke;
        papPoints = c.points;
        version++;
        return r.fulfill({ json: { data: { saved: true } } });
      }
      return r.fulfill({
        json: { data: { items: papHistory, next_cursor: "" } },
      });
    }
    if (url.pathname.endsWith("/pap"))
      return r.fulfill({
        json: {
          data: {
            points: papIssued ? papPoints * 2 : 0,
            events: papIssued ? 1 : 0,
            participations: papIssued ? 2 : 0,
            more: false,
            rows: papIssued
              ? [0, 1].map((i) => ({
                  event_id: "1",
                  character_id: String(123 + i),
                  account_id: "user",
                  name: i === 0 ? "Hajimi1" : "荣耀远航后勤与工业支援舰长",
                  title: "荣耀远航 · 周末联合舰队",
                  starts_at: "2026-09-15T11:30:00Z",
                  points: papPoints,
                }))
              : [],
          },
        },
      });
    if (url.pathname.endsWith("/battle"))
      return r.fulfill({
        json: {
          data: {
            ships: [
              {
                id: "1",
                ship_type_id: "587",
                ship_name: "裂谷级",
                solar_system_id: "30000142",
                solar_system_name: "吉他",
                observed_at: "2026-09-15T12:00:00Z",
                joined_at: "2026-09-15T11:40:00Z",
                state: "ready",
                fitting: {
                  ship_item_id: "999",
                  ship_type_id: "587",
                  ship_observed_at: "2026-09-15T12:00:10Z",
                  assets_observed_at: "2026-09-15T11:12:00Z",
                  assets_content_at: "2026-09-15T11:12:00Z",
                  items: [
                    {
                      type_id: "34",
                      name: "200mm 自动加农炮 II",
                      slot: "HiSlot0",
                      quantity: 3,
                      destroyed: 0,
                      dropped: 0,
                    },
                  ],
                },
              },
            ],
            losses: [
              {
                id: "100",
                ship_type_id: "587",
                ship_name: "裂谷级",
                solar_system_id: "30000142",
                solar_system_name: "吉他",
                occurred_at: "2026-09-15T12:30:00Z",
                state: "candidate",
                version: "1",
                items: [
                  {
                    type_id: "34",
                    name: "200mm 自动加农炮 II",
                    slot: "27",
                    quantity: 3,
                    destroyed: 1,
                    dropped: 2,
                  },
                ],
              },
            ],
            tasks: [
              {
                kind: "losses",
                state: "pending",
                reason: "",
                checked_at: "2026-09-15T12:35:00Z",
                next_due_at: "2026-09-15T13:35:00Z",
              },
            ],
            truncated: false,
          },
        },
      });
    if (url.pathname.endsWith("/review")) {
      expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
      expect(r.request().postDataJSON().state).toBe("confirmed");
      return r.fulfill({ json: { data: { saved: true } } });
    }
    if (url.pathname.endsWith("/context"))
      return r.fulfill({
        json: {
          data: {
            corporations: manage ? [{ id: "10", name: "Glory Navy" }] : [],
            characters: [
              { id: "123", name: "Hajimi1" },
              { id: "456", name: "荣耀远航后勤与工业支援舰长" },
            ],
          },
        },
      });
    if (url.pathname.endsWith("/capture")) {
      expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
      expect(r.request().postDataJSON().source_character_id).toBe("123");
      captured = true;
      version++;
      return r.fulfill({
        json: {
          data: {
            event: event(),
            recorded: 3,
            excluded: 2,
            excluded_external: 1,
            excluded_unbound: 1,
          },
        },
      });
    }
    if (url.pathname.endsWith("/manual")) {
      expect(r.request().postDataJSON().reason).toBe("后勤补录");
      version++;
      return r.fulfill({
        json: { data: { event: event(), recorded: 1, excluded: 0 } },
      });
    }
    if (url.pathname.endsWith("/events") && r.request().method() === "POST") {
      expect(r.request().postDataJSON().title).toBe("新活动");
      return r.fulfill({ json: { data: event() } });
    }
    if (url.pathname.endsWith("/events"))
      return r.fulfill({
        json: {
          data: {
            events: [
              event(),
              {
                ...event(),
                id: "2",
                title: "后勤运输",
                state: "closed",
                ends_at: "2026-09-15T13:45:00Z",
                participants: 12,
              },
            ],
            next_cursor: "",
          },
        },
      });
    if (url.pathname.endsWith("/events/1"))
      return r.fulfill({
        json: {
          data: {
            event: event(),
            entries: captured
              ? ["Hajimi1", "荣耀远航后勤与工业支援舰长", "Nuter Zero"].map(
                  (name, i) => ({
                    character_id: String(123 + i),
                    name,
                    account_id: i < 2 ? "user" : "other",
                    source: "fleet",
                    present: true,
                    recorded_at: "2026-09-15T12:00:00Z",
                    ship_type_id: "587",
                    solar_system_id: i === 0 ? "30000142" : null,
                    solar_system_name: i === 0 ? "吉他" : "",
                    location_observed_at:
                      i === 0 ? "2026-09-15T12:00:00Z" : null,
                    ship_name: "裂谷级",
                    losses: 0,
                    pap_points: papIssued ? papPoints : 0,
                  }),
                )
              : [],
          },
        },
      });
    if (url.pathname.endsWith("/online"))
      return r.fulfill({
        json: {
          data: {
            since: "2026-09-09T00:00:00+08:00",
            until: "2026-09-15T20:00:00+08:00",
            estimated: true,
            seconds: 27000,
            observed_characters: 2,
            days: Array.from({ length: 7 }, (_, i) => ({
              date: `2026-09-${String(9 + i).padStart(2, "0")}`,
              seconds: i < 2 ? 0 : i * 360,
              samples: i === 0 ? 0 : 200,
            })),
            members: [
              {
                user_id: "user",
                name: "Hajimi1",
                seconds: 27000,
                samples: 700,
                characters: [
                  {
                    id: "123",
                    name: "Hajimi1",
                    state: "online",
                    observed_at: "2026-09-15T12:00:00Z",
                  },
                  {
                    id: "456",
                    name: "荣耀远航后勤与工业支援舰长",
                    state: "unknown",
                    observed_at: null,
                  },
                ],
              },
            ],
          },
        },
      });
    return r.fulfill({
      status: 404,
      json: { error: { message: "记录不存在或无权访问" } },
    });
  });
}
test("English attendance layout", async ({ page }, info) => {
  await setup(page);
  await page.goto("/attendance");
  await englishLayout(
    page,
    "Corporation attendance",
    `attendance-${info.project.name}`,
  );
});

test("organizer captures fleet and corrects attendance", async ({ page }) => {
  await setup(page);
  await page.goto("/attendance");
  await page.getByRole("button", { name: /荣耀远航 · 周末联合舰队/ }).click();
  await page.getByRole("button", { name: "舰队点名", exact: true }).click();
  await expect(page.getByRole("status")).toHaveText(
    "已读取 3 名军团角色，跳过外团 1 名、未绑定 1 名",
  );
  await expect(
    page.getByText("出勤人数", { exact: true }).locator(".."),
  ).toContainText("2");
  await page.getByRole("button", { name: "补录", exact: true }).click();
  await page.getByLabel("角色 ID", { exact: true }).fill("789");
  await page.getByLabel("修订原因").fill("后勤补录");
  await page.getByRole("button", { name: "保存", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
});
test("create activity and dialog keyboard navigation", async ({ page }) => {
  await setup(page);
  await page.goto("/attendance");
  await page.getByRole("button", { name: "创建活动", exact: true }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.getByLabel("活动名称").fill("新活动");
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "创建活动", exact: true })
    .click();
  await expect(page.getByRole("heading", { name: "出勤名单" })).toBeVisible();
});
test("member cannot see organizer controls and online distinguishes unknown", async ({
  page,
}) => {
  await setup(page, false);
  await page.goto("/attendance");
  await expect(
    page.getByRole("button", { name: "创建活动", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("tab", { name: "在线时长", exact: true }).click();
  await expect(
    page.getByRole("img", { name: /每日估算在线小时/ }),
  ).toBeVisible();
  await expect(page.getByText("未知", { exact: true })).toBeVisible();
  const toggle = page.getByRole("button", { name: "显示每日明细" });
  await toggle.focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("table", { name: "每日在线时长" })).toBeVisible();
  await expect(
    page.getByRole("row").filter({ hasText: "2026-09-09" }),
  ).toContainText("—");
  await expect(
    page.getByRole("row").filter({ hasText: "2026-09-10" }),
  ).toContainText("0 h");
});
test("attendance layouts 1440 375 320 and enlarged text", async ({
  page,
}, info) => {
  await setup(page);
  const dir = fileURLToPath(
    new URL("../../docs/ui/reviews/attendance/", import.meta.url),
  );
  mkdirSync(dir, { recursive: true });
  for (const width of [1440, 375, 320]) {
    await page.setViewportSize({ width, height: 950 });
    await page.goto("/attendance");
    await expect(page.getByText("后勤运输", { exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: /后勤运输/ }).locator('.attendance-event-end time')).toHaveAttribute('datetime', '2026-09-15T13:45:00Z');
    await expect(page.getByRole("button", { name: /荣耀远航 · 周末联合舰队/ }).locator('.attendance-event-end')).toContainText('—');
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: `${dir}/${info.project.name}-events-${width}.png`,
      fullPage: true,
    });
    await page.getByRole("button", { name: /荣耀远航 · 周末联合舰队/ }).click();
    await page.getByRole("button", { name: "舰队点名", exact: true }).click();
    await expect(page.getByText("Nuter Zero", { exact: true })).toBeVisible();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: `${dir}/${info.project.name}-roster-${width}.png`,
      fullPage: true,
    });
    await page.getByRole("tab", { name: "在线时长", exact: true }).click();
    await expect(
      page.getByRole("img", { name: /每日估算在线小时/ }),
    ).toBeVisible();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: `${dir}/${info.project.name}-online-${width}.png`,
      fullPage: true,
    });
  }
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.addStyleTag({ content: ":root{font-size:200%}" });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
});

test("ship fitting and loss evidence remain readable on narrow screens", async ({
  page,
}, testInfo) => {
  await setup(page);
  await page.goto("/attendance?event=1");
  await page.getByRole("button", { name: "舰队点名", exact: true }).click();
  await expect(page.getByText("点名星系 吉他", { exact: true })).toBeVisible();
  await expect(
    page.getByText("点名星系 未记录", { exact: true }).first(),
  ).toBeVisible();
  await page.getByRole("button", { name: "舰船与损失" }).first().click();
  const ships = page.getByRole("region", { name: "Hajimi1 的舰船快照" });
  await expect(ships.getByText("点名星系 吉他", { exact: true })).toBeVisible();
  await ships.locator("summary").click();
  await expect(ships.getByText("200mm 自动加农炮 II")).toBeVisible();
  await expect(ships.getByText(/资产快照/)).toBeVisible();
  const losses = page.getByRole("region", { name: "Hajimi1 的活动损失" });
  await expect(
    losses.getByText("损失星系 吉他", { exact: true }),
  ).toBeVisible();
  await losses.locator("summary").click();
  await expect(losses.getByText("毁 1 · 掉 2")).toBeVisible();
  await losses.getByRole("button", { name: "计入活动", exact: true }).click();
  await captureDialogLayout(page, "计入活动", `${testInfo.project.name}-loss-confirm`);
  await page.getByRole("dialog").getByRole("textbox", { name: "确认原因" }).fill("本次活动损失");
  await page.getByRole("dialog").getByRole("button", { name: "确认计入", exact: true }).click();
  await expect(page.getByRole("dialog").getByRole("textbox", { name: "确认原因" })).toHaveCount(
    0,
  );
  await expect(losses.getByRole("button", { name: "计入活动", exact: true })).toBeFocused();
  await losses.getByRole("button", {name:"排除",exact:true}).click();
  await captureDialogLayout(page,"排除",`${testInfo.project.name}-loss-exclude`);
  await expect(page.getByRole("dialog").getByRole("button", {name:"取消",exact:true})).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(losses.getByRole("button", {name:"排除",exact:true})).toBeFocused();
  await page.setViewportSize({
    width: testInfo.project.name === "mobile" ? 375 : 1440,
    height: 1100,
  });
  await expect
    .poll(() =>
      page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    )
    .toBe(true);
  const dir = fileURLToPath(
    new URL("../../docs/ui/reviews/attendance", import.meta.url),
  );
  mkdirSync(dir, { recursive: true });
  await page.screenshot({
    path: `${dir}/${testInfo.project.name}-battle.png`,
    fullPage: true,
  });
  await page.setViewportSize({ width: 320, height: 900 });
  await expect
    .poll(() =>
      page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    )
    .toBe(true);
});

test("PAP issues per character, sums alts, and revokes with history", async ({
  page,
}, testInfo) => {
  await setup(page, true, true);
  await page.goto("/attendance?event=1");
  await page.getByRole("button", { name: "发放军团 PAP", exact: true }).click();
  await page.getByRole("spinbutton", { name: "每角色分值" }).fill("2");
  await expect(page.getByText("3 个角色 × 2 分 = 6 分")).toBeVisible();
  await captureDialogLayout(page, "发放军团 PAP", `${testInfo.project.name}-pap-issue`);
  await page.getByLabel("发分说明").fill("集结奖励");
  await page.getByRole("button", { name: "确认发放", exact: true }).click();
  await expect(page.getByRole("button", { name: "更正分值" })).toBeVisible();
  await expect(
    page.getByRole("button", { name: "先撤销发分再重开" }),
  ).toBeDisabled();
  await page.getByRole("button", { name: "明细", exact: true }).click();
  await expect(page.getByText("余额 2").first()).toBeVisible();
  await page.setViewportSize({
    width: testInfo.project.name === "mobile" ? 375 : 1440,
    height: 1100,
  });
  const dir = fileURLToPath(
    new URL("../../docs/ui/reviews/attendance", import.meta.url),
  );
  await page.screenshot({
    path: `${dir}/${testInfo.project.name}-pap-event.png`,
    fullPage: true,
  });
  await page.getByRole("tab", { name: "军团 PAP", exact: true }).click();
  await expect(page.getByRole("heading", { name: "军团 PAP 明细" })).toBeVisible();
  await expect(page.locator(".attendance-metric").first()).toContainText("4");
  await page.screenshot({
    path: `${dir}/${testInfo.project.name}-pap-report.png`,
    fullPage: true,
  });
  await page.setViewportSize({ width: 320, height: 1000 });
  await expect
    .poll(() =>
      page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    )
    .toBe(true);
  await page.getByRole("button").filter({ hasText: "Hajimi1" }).click();
  await page.getByRole("button", { name: "撤销军团 PAP", exact: true }).click();
  await captureDialogLayout(page, "撤销军团 PAP", `${testInfo.project.name}-pap-revoke`);
  await page.getByLabel("撤销原因").fill("名单修订");
  await page.getByRole("button", { name: "确认撤销", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "重新开启", exact: true }),
  ).toBeEnabled();
  await page.getByRole("button", { name: "明细", exact: true }).click();
  await expect(page.getByText("余额 0").first()).toBeVisible();
});
test("administrator previews and converts pending PAP once", async ({
  page,
}, testInfo) => {
  await setup(page, true, true);
  let converted = false,
    requests = 0;
  await page.route("**/api/v1/attendance/events/1/conversion", async (r) => {
    if (r.request().method() === "POST") {
      requests++;
      const body = r.request().postDataJSON();
      expect(body.token).toBe("a".repeat(64));
      expect(body.reason).toBe("九月集结兑换");
      expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
      converted = true;
      return r.fulfill({ json: { data: { saved: true } } });
    }
    return r.fulfill({
      json: {
        data: {
          token: "a".repeat(64),
          mode: "manual",
          points: 6,
          converted: converted ? 6 : 0,
          pending: converted ? 0 : 6,
          coins_minor: converted ? 0 : 1500,
          characters: converted ? 0 : 3,
        },
      },
    });
  });
  await page.goto("/attendance?event=1");
  await page.getByRole("button", { name: "发放军团 PAP", exact: true }).click();
  await page.getByRole("spinbutton", { name: "每角色分值" }).fill("2");
  await page.getByLabel("发分说明").fill("集结");
  await page.getByRole("button", { name: "确认发放", exact: true }).click();
  await page.getByRole("button", { name: "兑换果壳币", exact: true }).click();
  await expect(page.getByText("已兑换 0 分 · 待兑换 6 分")).toBeVisible();
  await expect(page.getByText("3 个角色 · 本次发放 15 果壳币")).toBeVisible();
  await page.getByLabel("兑换说明").fill("九月集结兑换");
  await captureDialogLayout(page, "兑换果壳币", `${testInfo.project.name}-pap-coins`);
  for (const width of [1440, 375, 320]) {
    await page.setViewportSize({ width, height: 1000 });
    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth > window.innerWidth,
    );
    expect(overflow).toBe(false);
    await page.screenshot({
      path: `../.local/welfare/conversion-${testInfo.project.name}-${width}.png`,
      fullPage: true,
    });
  }
  await page.getByRole("button", { name: "确认兑换", exact: true }).click();
  await expect(page.getByText("已兑换", { exact: true })).toBeVisible();
  expect(requests).toBe(1);
  await page.getByRole("button", { name: "兑换果壳币", exact: true }).click();
  await expect(page.getByText("本次积分已全部兑换")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "确认兑换", exact: true }),
  ).toHaveCount(0);
});

test("PAP dialog retains failed attempt and blocks dismissal while pending", async ({ page }) => {
  await setup(page, true, true);
  const bodies: unknown[] = [];
  let release!: () => void;
  const gate = new Promise<void>((resolve) => { release = resolve; });
  await page.route("**/api/v1/attendance/events/1/pap", async (route) => {
    if (route.request().method() !== "POST") return route.fallback();
    bodies.push(route.request().postDataJSON());
    if (bodies.length === 1) return route.fulfill({status: 503, json: {error: {code: "unavailable", message: "稍后重试"}}});
    await gate;
    return route.fallback();
  });
  await page.goto("/attendance?event=1");
  await page.getByRole("button", {name: "发放军团 PAP", exact: true}).click();
  const dialog = page.getByRole("dialog", {name: "发放军团 PAP", exact: true});
  const reason = dialog.getByLabel("发分说明");
  await reason.fill("   ");
  await expect(dialog.getByRole("button", {name: "确认发放", exact: true})).toBeDisabled();
  await reason.fill("集结奖励");
  await dialog.getByRole("button", {name: "确认发放", exact: true}).click();
  await expect(dialog.getByRole("alert")).toContainText("稍后重试");
  await expect(reason).toHaveValue("集结奖励");
  await dialog.getByRole("button", {name: "确认发放", exact: true}).click();
  try {
    await expect(dialog).toHaveAttribute("aria-busy", "true");
    await expect(reason).toBeDisabled();
    await expect(dialog.getByRole("button", {name: "取消", exact: true})).toBeDisabled();
    await expect(dialog.getByRole("button", {name: "关闭", exact: true})).toBeDisabled();
    await page.keyboard.press("Escape");
    await page.mouse.click(1, 1);
    await expect(dialog).toBeVisible();
    await expect.poll(() => bodies.length).toBe(2);
    expect(bodies[1]).toEqual(bodies[0]);
  } finally { release(); }
  await expect(dialog).toHaveCount(0);
  await expect(page.getByRole("button", {name: "更正分值", exact: true})).toBeFocused();
});

test("PAP English dialog layout", async ({ page }, testInfo) => {
  await setup(page, true, true);
  await page.goto("/attendance?event=1");
  await page.getByRole("button", {name: "切换为英文", exact: true}).click();
  await page.getByRole("button", {name: "Award corporation PAP", exact: true}).click();
  await captureDialogLayout(page, "Award corporation PAP", `${testInfo.project.name}-pap-en`);
  await page.keyboard.press("Escape");
  await expect(page.getByRole("button", {name: "Award corporation PAP", exact: true})).toBeFocused();
});

test("PAP member has own summary without issue controls", async ({ page }) => {
  await setup(page, false, true);
  await page.goto("/attendance?event=1");
  await expect(
    page.getByRole("button", { name: "发放军团 PAP", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("tab", { name: "军团 PAP", exact: true }).click();
  await expect(page.getByText("暂无军团 PAP")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "兑换果壳币", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("combobox", { name: "军团 PAP 范围" }),
  ).toHaveCount(0);
});

test("PAP manager can distinguish own and corporation totals", async ({ page }) => {
  await setup(page, true, true);
  await page.goto("/attendance?view=pap");
  const scope = page.getByRole("combobox", { name: "军团 PAP 范围" });
  await expect(scope).toContainText("我的积分");
  await expect(page.locator(".attendance-metric").first()).toContainText("我的积分");
  await scope.click();
  const corporationRequest = page.waitForRequest((request) => {
    const url = new URL(request.url());
    return url.pathname.endsWith("/attendance/pap") && url.searchParams.get("corporation_id") === "10";
  });
  await page.getByRole("option", { name: "Glory Navy · 全团" }).click();
  await corporationRequest;
  await expect(page.locator(".attendance-metric").first()).toContainText("全团积分");
});

stateLayouts({ name: "attendance", setup: setup, path: "/attendance", endpoint: "**/api/v1/attendance/events?**", empty: { events: [], next_cursor: "" }, emptyText: /暂无活动记录|No events/ });

stateLayouts({ name: "online", setup: setup, path: "/attendance?view=online", endpoint: "**/api/v1/attendance/context" });

stateLayouts({ name: "pap", setup: setup, path: "/attendance?view=pap", endpoint: "**/api/v1/attendance/context" });
