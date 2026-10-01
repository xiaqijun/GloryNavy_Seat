import { test, expect } from "@playwright/test";
import { mkdirSync } from "node:fs";

for (const language of ["zh-CN", "en"]) {
  test(`contract appraisal ${language}: scope, pages, sides and failure recovery`, async ({
    page,
  }, info) => {
    const en = language === "en";
    const text = (zh: string, english: string) => (en ? english : zh);
    let rejected = true;
    const requests: unknown[] = [];
    const entity = { id: "0", name: "", category: "" };
    const contract = (id: string, changes = {}) => ({
      id,
      title: `Ishtar fitting ${id}`,
      type: "item_exchange",
      status: "outstanding",
      availability: "private",
      for_corporation: false,
      issuer: { ...entity, id: "123", name: "Pilot" },
      assignee: entity,
      acceptor: entity,
      start: entity,
      end: entity,
      price: "200000000.00",
      reward: null,
      collateral: null,
      buyout: null,
      volume: null,
      days_to_complete: null,
      date_issued: "2026-09-19T10:00:00Z",
      date_expired: "2099-01-01T00:00:00Z",
      date_accepted: "",
      date_completed: "",
      checked_at: "2026-09-19T10:00:00Z",
      ...changes,
    });
    await page.route("https://images.evetech.net/**", (r) => r.abort());
    await page.route("**/api/v1/**", async (r) => {
      const url = new URL(r.request().url()),
        p = url.pathname;
      let data: unknown = {};
      if (p === "/api/v1/modules")
        data = ["system", "identity", "eve", "access", "market"].map((id) => ({
          id,
          api_version: 1,
          version: "0.1.0",
        }));
      if (p === "/api/v1/identity/session")
        data = {
          authenticated: true,
          session: {
            user_id: "00000000-0000-4000-8000-000000000001",
            character: { id: "123", name: "Pilot" },
            csrf_token: "test",
            expires_at: "2099-01-01T00:00:00Z",
          },
        };
      if (p === "/api/v1/access/me")
        data = {
          administrator: true,
          can_manage: true,
          can_manage_sync: true,
          site_roles: [],
          characters: [],
        };
      if (p === "/api/v1/market/settings")
        data = {
          settings: { ratio_bps: 8000, version: 1 },
          administrator: true,
        };
      if (p === "/api/v1/eve/contracts/owners")
        data = {
          owners: [
            { kind: "character", id: "123", name: "Pilot" },
            { kind: "corporation", id: "456", name: "Glory Navy" },
          ],
        };
      if (/\/contracts\/(character|corporation)\/\d+$/.test(p)) {
        expect(url.searchParams.get("status")).toBe("outstanding");
        data = url.searchParams.get("before")
          ? { items: [contract("88")], next_cursor: "" }
          : {
              items: [
                contract("99"),
                contract("98", { status: "finished" }),
                contract("97", { date_expired: "2000-01-01T00:00:00Z" }),
                contract("96", { type: "courier" }),
              ],
              next_cursor: "90",
            };
      }
      if (p === "/api/v1/market/estimate-contract") {
        expect(r.request().headers()["x-csrf-token"]).toBe("test");
        const body = r.request().postDataJSON();
        requests.push(body);
        expect(body).toEqual({
          owner_kind: "corporation",
          owner_id: "456",
          contract_id: "88",
          side: "requested",
        });
        if (rejected) {
          rejected = false;
          return r.fulfill({
            status: 409,
            json: {
              error: {
                code: "contract_not_appraisable",
                message: "Items pending",
              },
            },
          });
        }
        data = {
          lines: [
            {
              input: "Tritanium",
              name: "Tritanium",
              quantity: "1000",
              type_id: "34",
              status: "ready",
              buy: "4000.00",
              mid: "5000.00",
              sell: "6000.00",
              observed_at: null,
            },
          ],
          totals: { buy: "4000.00", mid: "5000.00", sell: "6000.00" },
          adjusted: { buy: "3200.00", mid: "4000.00", sell: "4800.00" },
          ratio_bps: 8000,
          complete: true,
        };
      }
      return r.fulfill({ json: { data } });
    });
    await page.goto(`/appraisal?lang=${language}`);
    const paste = page.getByLabel(text("物品清单", "Item list"));
    await paste.fill("Keep this list");
    const opener = page.getByRole("button", {
      name: text("从合同估价", "Appraise a contract"),
      exact: true,
    });
    await opener.click();
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByRole("radio")).toHaveCount(1);
    await dialog
      .getByRole("combobox", { name: text("合同来源", "Contract source") })
      .click();
    await page.getByRole("option", { name: "Glory Navy", exact: true }).click();
    await dialog
      .getByRole("button", { name: text("加载更多", "Load more") })
      .click();
    await expect(dialog.getByRole("radio")).toHaveCount(2);
    await dialog.getByRole("radio").last().focus();
    await page.keyboard.press("Space");
    await dialog
      .getByRole("combobox", {
        name: text("估价物品范围", "Items to appraise"),
      })
      .click();
    await page
      .getByRole("option", {
        name: text("要求物品", "Requested items"),
        exact: true,
      })
      .click();
    mkdirSync("../.local/market-contracts", { recursive: true });
    for (const width of info.project.name === "desktop" ? [1440] : [375, 320]) {
      await page.setViewportSize({ width, height: 900 });
      await page.screenshot({
        path: `../.local/market-contracts/${language}-${width}.png`,
      });
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth),
      ).toBeLessThanOrEqual(width);
      expect(
        await dialog
          .locator(".ui-modal-body")
          .evaluate((e) => e.scrollWidth <= e.clientWidth),
      ).toBe(true);
      const box = await dialog.boundingBox();
      expect(box!.x).toBeGreaterThanOrEqual(0);
      expect(box!.x + box!.width).toBeLessThanOrEqual(width);
    }
    const submit = dialog.getByRole("button", {
      name: text("估价", "Appraise"),
      exact: true,
    });
    await submit.click();
    await expect(dialog.getByRole("alert")).toHaveText("Items pending");
    await expect(dialog.getByRole("radio").last()).toBeChecked();
    await submit.click();
    await expect(dialog).toHaveCount(0);
    await expect(
      page.getByText("5,000.00", { exact: true }).first(),
    ).toBeVisible();
    await expect(
      page.getByText(new RegExp("Ishtar fitting 88 #88")),
    ).toBeVisible();
    await expect(paste).toHaveValue("Keep this list");
    await expect(opener).toBeFocused();
    expect(requests).toHaveLength(2);
  });
}
