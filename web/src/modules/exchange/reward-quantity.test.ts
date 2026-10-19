import { describe, expect, it } from "vitest";
import { maxRewardUnits, scaledReward } from "./reward-quantity";
import { isOrders, type Reward, type Shop } from "./rewards-api";
import { isPhysical, isPhysicalSnapshot } from "./catalog-api";

const reward: Reward = {
  id: "1",
  type_id: "34",
  name: "Pack",
  quantity: 1,
  isk_value: 401,
  coins_minor: 5,
  stock: 9,
  enabled: true,
  version: "1",
  content: {
    fittings: [],
    items: [{ type_id: "34", quantity: 1000000 }],
    isk_minor: 12345,
  },
};
const shop: Shop = {
  admin: false,
  version: "1",
  isk_per_coin: 10000,
  earned_minor: 10,
  available_minor: 10,
  reserved_minor: 0,
  spent_minor: 0,
  rewards: [reward],
  next_cursor: "",
};
describe("reward quantity", () => {
  it("bounds by whole affordable packs, stock and safe totals", () => {
    expect(maxRewardUnits(reward, shop)).toBe(2);
    expect(maxRewardUnits({ ...reward, stock: 1 }, shop)).toBe(1);
    expect(maxRewardUnits(reward, { ...shop, available_minor: -1 })).toBe(0);
    expect(maxRewardUnits({ ...reward, coins_minor: 0 }, shop)).toBe(0);
    expect(
      maxRewardUnits(
        { ...reward, content: { fittings: [], items: [], isk_minor: 1e14 } },
        { ...shop, available_minor: 1e9 },
      ),
    ).toBe(9);
  });
  it("scales all components without changing the quote", () => {
    const scaled = scaledReward(reward.content, 2)!;
    expect(scaled.items[0].quantity).toBe(2000000);
    expect(scaled.isk_minor).toBe(24690);
    expect(reward.content!.items[0].quantity).toBe(1000000);
    expect(isPhysical(scaled)).toBe(false);
    expect(isPhysicalSnapshot(scaled)).toBe(true);
  });
  it("reads expanded and legacy orders, rejects unsafe units", () => {
    const order = {
      id: "1",
      version: "1",
      type_id: "34",
      name: "Pack",
      quantity: 2,
      units: 2,
      recipient_id: "1",
      recipient_name: "Pilot",
      coins_minor: 10,
      isk_per_coin: 10000,
      isk_value: 802,
      state: "pending",
      note: "",
      created_at: "2026-10-19T00:00:00Z",
      content: scaledReward(reward.content, 2),
    };
    const valid = (o: unknown) => isOrders({ items: [o], next_cursor: "" });
    expect(valid(order)).toBe(true);
    expect(valid({ ...order, units: undefined })).toBe(true);
    for (const units of [0, -1, 1.5, 1000001])
      expect(valid({ ...order, units })).toBe(false);
  });
});
