import { describe, it, expect } from "vitest";
import { isBattle } from "./battle-api";
describe("battle evidence contract", () => {
  it("keeps empty evidence distinct from zero loss", () => {
    expect(
      isBattle({ ships: [], losses: [], tasks: [], truncated: false }),
    ).toBe(true);
    expect(
      isBattle({ ships: [], losses: 0, tasks: [], truncated: false }),
    ).toBe(false);
  });
  it("rejects invalid loss states and amounts", () => {
    const loss = {
      id: "1",
      ship_type_id: "587",
      ship_name: "裂谷级",
      solar_system_id: "30000142",
      occurred_at: "2026-09-15T12:00:00Z",
      state: "candidate",
      version: "1",
      items: [
        {
          type_id: "34",
          name: "物品",
          slot: "27",
          quantity: 3,
          destroyed: 1,
          dropped: 2,
        },
      ],
    };
    const data = { ships: [], losses: [loss], tasks: [], truncated: false };
    expect(isBattle(data)).toBe(true);
    expect(isBattle({ ...data, losses: [{ ...loss, state: "paid" }] })).toBe(
      false,
    );
    expect(
      isBattle({
        ...data,
        losses: [{ ...loss, items: [{ ...loss.items[0], dropped: -1 }] }],
      }),
    ).toBe(false);
  });
});
