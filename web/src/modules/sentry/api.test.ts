import { afterEach, describe, expect, it, vi } from "vitest";
import { consumptions } from "./api";

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
