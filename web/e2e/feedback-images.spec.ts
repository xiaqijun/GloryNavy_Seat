import { test, expect, type Page } from "@playwright/test";

const png = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a2ioAAAAASUVORK5CYII=",
  "base64",
);
const image = { name: "screenshot.png", mimeType: "image/png", buffer: png };
async function setup(page: Page, language: string, retry = false) {
  const account = "00000000-0000-4000-8000-000000000001";
  const ticket = {
    id: "1",
    account_id: account,
    author_name: "Pilot",
    kind: "bug",
    title: "合同提交失败",
    body: "提交时出现错误",
    status: "submitted",
    revision: 1,
    created_at: "2026-10-19T00:00:00Z",
    updated_at: "2026-10-19T00:00:00Z",
    images: [] as { id: string; mime: string }[],
  };
  const messages: Record<string, unknown>[] = [];
  const attempts: Record<string, unknown>[] = [];
  await page.addInitScript(
    (l) => localStorage.setItem("glorynavy.locale", l),
    language,
  );
  await page.route("https://images.evetech.net/**", (r) => r.abort());
  await page.route("**/api/v1/**", async (r) => {
    const p = new URL(r.request().url()).pathname;
    let data: unknown = {};
    if (p === "/api/v1/modules")
      data = ["system", "identity", "eve", "access", "feedback"].map((id) => ({
        id,
        api_version: 1,
        version: "0.1.0",
      }));
    if (p === "/api/v1/identity/session")
      data = {
        authenticated: true,
        session: {
          user_id: account,
          character: { id: "123", name: "Pilot" },
          csrf_token: "csrf",
          expires_at: "2099-01-01T00:00:00Z",
        },
      };
    if (p === "/api/v1/access/me")
      data = {
        administrator: false,
        can_manage: false,
        can_manage_sync: false,
        site_roles: [],
        characters: [],
      };
    if (/\/feedback\/tickets\/\d+\/images\/\d+$/.test(p))
      return r.fulfill({ contentType: "image/png", body: png });
    if (p === "/api/v1/feedback/tickets") {
      if (r.request().method() === "POST") {
        const b = r.request().postDataJSON();
        attempts.push(b);
        expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
        if (retry && attempts.length === 1)
          return r.fulfill({
            status: 503,
            json: { error: { message: "Temporary error" } },
          });
        Object.assign(ticket, {
          title: b.title,
          body: b.body,
          images: b.images.map((i: { mime: string }, n: number) => ({
            id: String(n + 1),
            mime: i.mime,
          })),
        });
        data = ticket;
      } else
        data = { tickets: [ticket], next_before: "", administrator: false };
    }
    if (p === "/api/v1/feedback/tickets/1")
      data = { ticket, messages, administrator: false };
    if (p === "/api/v1/feedback/tickets/1/messages") {
      const b = r.request().postDataJSON();
      attempts.push(b);
      ticket.revision++;
      messages.push({
        id: "1",
        body: b.body,
        status: "",
        staff: false,
        author_name: "Pilot",
        created_at: ticket.created_at,
        images: b.images.map((i: { mime: string }, n: number) => ({
          id: String(n + 10),
          mime: i.mime,
        })),
      });
      data = ticket;
    }
    return r.fulfill({ json: { data } });
  });
  return attempts;
}

for (const language of ["zh-CN", "en"]) {
  const t = (zh: string, en: string) => (language === "en" ? en : zh);
  test(`screenshots create retry and reply ${language}`, async ({
    page,
  }, info) => {
    const attempts = await setup(page, language, true);
    await page.goto("/feedback");
    await page
      .getByRole("button", {
        name: t("提交反馈", "Submit feedback"),
        exact: true,
      })
      .click();
    const dialog = page.getByRole("dialog");
    await dialog
      .getByLabel(t("标题", "Title"), { exact: true })
      .fill("截图反馈");
    await dialog
      .getByLabel(t("详细描述", "Description"))
      .fill("操作后发生错误");
    await dialog.locator('input[type="file"]').setInputFiles([image, image]);
    await expect(dialog.locator(".feedback-image-draft")).toHaveCount(2);
    await dialog
      .getByRole("button", { name: t("移除截图 1", "Remove screenshot 1") })
      .click();
    await expect(dialog.locator(".feedback-image-draft")).toHaveCount(1);
    await dialog
      .getByRole("button", { name: t("提交", "Submit"), exact: true })
      .click();
    await expect(dialog.getByRole("alert")).toHaveText("Temporary error");
    await expect(dialog.locator(".feedback-image-draft")).toHaveCount(1);
    await dialog
      .getByRole("button", { name: t("提交", "Submit"), exact: true })
      .click();
    await expect(
      dialog.getByRole("heading", { name: "截图反馈", exact: true }),
    ).toBeVisible();
    expect(attempts[0]).toEqual(attempts[1]);
    await expect(
      dialog.getByRole("link", { name: t("查看截图 1", "View screenshot 1") }),
    ).toHaveAttribute("href", "/api/v1/feedback/tickets/1/images/1");
    await dialog
      .getByLabel(t("补充说明", "Additional details"))
      .fill("补充截图");
    await dialog.locator('input[type="file"]').setInputFiles(image);
    await dialog
      .getByRole("button", { name: t("发送回复", "Send reply") })
      .click();
    await expect(dialog.locator(".feedback-messages img")).toHaveCount(1);
    await expect(dialog.locator(".feedback-image-draft")).toHaveCount(0);
    await expect(
      dialog.locator(".feedback-images img").first(),
    ).toHaveJSProperty("naturalWidth", 1);
    await page.screenshot({
      path: info.outputPath("screenshots.png"),
      fullPage: true,
    });
    await expect(page.locator("body")).toHaveJSProperty(
      "scrollWidth",
      await page.evaluate(() => innerWidth),
    );
  });

  test(`screenshots limits and paste ${language}`, async ({ page }) => {
    await setup(page, language);
    await page.goto("/feedback?ticket=1");
    const dialog = page.getByRole("dialog"),
      input = dialog.locator('input[type="file"]');
    await input.setInputFiles({
      name: "bad.svg",
      mimeType: "image/svg+xml",
      buffer: Buffer.from("<svg/>"),
    });
    await expect(dialog.getByRole("alert")).toBeVisible();
    await expect(dialog.locator(".feedback-image-draft")).toHaveCount(0);
    await input.setInputFiles({ ...image, buffer: Buffer.alloc(2_000_001) });
    await expect(dialog.getByRole("alert")).toBeVisible();
    await input.setInputFiles([image, image, image, image]);
    await expect(dialog.locator(".feedback-image-draft")).toHaveCount(0);
    await input.setInputFiles([image, image]);
    await expect(dialog.getByRole("alert")).toHaveCount(0);
    await dialog.locator("textarea").evaluate((el, base64) => {
      const bytes = Uint8Array.from(atob(base64), (c) => c.charCodeAt(0));
      const data = new DataTransfer();
      data.items.add(new File([bytes], "paste.png", { type: "image/png" }));
      el.dispatchEvent(
        new ClipboardEvent("paste", {
          bubbles: true,
          cancelable: true,
          clipboardData: data,
        }),
      );
    }, png.toString("base64"));
    await expect(dialog.locator(".feedback-image-draft")).toHaveCount(3);
    await expect(
      dialog.getByRole("button", { name: t("添加截图", "Add screenshots") }),
    ).toBeDisabled();
    await dialog
      .getByRole("button", { name: t("移除截图 2", "Remove screenshot 2") })
      .click();
    await expect(
      dialog.getByRole("button", { name: t("添加截图", "Add screenshots") }),
    ).toBeEnabled();
  });
}
