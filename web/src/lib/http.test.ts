import { afterEach, expect, it, vi } from "vitest";
afterEach(() => {
  vi.unstubAllGlobals();
  vi.resetModules();
});
it("sends the selected locale without altering CSRF, body, credentials or cancellation", async () => {
  vi.stubGlobal("window", { location: { href: "http://localhost/?lang=en" } });
  const fetch = vi.fn().mockResolvedValue(new Response("{}"));
  vi.stubGlobal("fetch", fetch);
  const { apiFetch } = await import("./http");
  const signal = new AbortController().signal;
  const headers = new Headers({
    "X-CSRF-Token": "original",
    "Content-Type": "application/json",
  });
  const body = '{"description":"用户原文","id":"9007199254740993"}';
  await apiFetch("/api/v1/welfare/commands", {
    method: "POST",
    headers,
    signal,
    credentials: "same-origin",
    body,
  });
  const [url, options] = fetch.mock.calls[0];
  expect(url).toBe("/api/v1/welfare/commands");
  expect(options).toMatchObject({
    method: "POST",
    signal,
    credentials: "same-origin",
    body,
  });
  expect(options.headers.get("Accept-Language")).toBe("en");
  expect(options.headers.get("X-CSRF-Token")).toBe("original");
  expect(options.headers.get("Content-Type")).toBe("application/json");
  expect(headers.has("Accept-Language")).toBe(false);
});
