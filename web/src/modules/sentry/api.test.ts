import { afterEach, describe, expect, it, vi } from "vitest";
import { consumptions, updateConnection } from "./api";

afterEach(() => vi.unstubAllGlobals());

const interval = {
  id: "1", grant_id: "g", interval_id: "raw-usage", system_id: "30000142",
  started_at: "2026-10-24T00:00:00Z", ended_at: "2026-10-24T00:00:01Z",
  duration_seconds: 1, coins_minor: 0, state: "settled", unit_seconds: 3600,
  unit_price_minor: 200, price_version: "hourly-v13", expires_at: "2026-10-25T00:00:00Z",
};

describe("cumulative alert consumption responses", () => {
  it("keeps zero-minor settled intervals in a valid consumption page", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(Response.json({ data: {
      items: [interval, { ...interval, id: "2", coins_minor: 1 }],
      next_cursor: "", as_of: "2026-10-24T00:01:00Z",
    } })));
    const page = await consumptions({});
    expect(page.items.map((item) => item.coins_minor)).toEqual([0, 1]);
    expect(page.items[0].duration_seconds).toBe(1);
  });

  it.each([-1, 0.5, Number.MAX_SAFE_INTEGER + 1])("rejects invalid charge %s", async (coins) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(Response.json({ data: {
      items: [{ ...interval, coins_minor: coins }], next_cursor: "", as_of: "2026-10-24T00:01:00Z",
    } })));
    await expect(consumptions({})).rejects.toThrow();
  });
});

it("submits a connection token only in the protected settings write", async () => {
  const origin = "https://sentry.example.org";
  const token = "a".repeat(64);
  const revision = 2;
  const settings = { origin, revision: 3, token_configured: true };
  const fetchMock = vi.fn().mockResolvedValue(Response.json({ data: settings }));
  vi.stubGlobal("fetch", fetchMock);

  await expect(updateConnection("csrf", { origin, revision, service_token: token })).resolves.toEqual(settings);
  const [path, request] = fetchMock.mock.calls[0];
  expect(path).toBe("/api/v1/sentry/connection");
  expect(request.method).toBe("PUT");
  expect(request.headers.get("X-CSRF-Token")).toBe("csrf");
  expect(JSON.parse(request.body)).toEqual({ origin, revision, service_token: token });
});
