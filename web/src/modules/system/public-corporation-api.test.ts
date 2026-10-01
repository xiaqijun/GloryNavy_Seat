import { describe, it, expect } from "vitest";
import { isPublicCorporation } from "./public-corporation-api";
const profile = {
  corporation_id: 98530802,
  name: "Glory Navy",
  ticker: "G.N.V",
  member_count: 379,
  date_founded: "2017-09-27T11:40:29Z",
  updated_at: "2026-09-23T00:00:00Z",
  stale: false,
};
describe("public corporation snapshot", () => {
  it("rejects missing/invalid data instead of displaying zero", () => {
    expect(isPublicCorporation(profile)).toBe(true);
    for (const patch of [
      { member_count: undefined },
      { member_count: -1 },
      { member_count: 1.5 },
      { corporation_id: 1 },
      { date_founded: "unknown" },
      { updated_at: "" },
      { alliance: { id: 2, name: "A", ticker: "A", corporation_count: -1 } },
    ])
      expect(isPublicCorporation({ ...profile, ...patch })).toBe(false);
  });
  it("accepts a real zero and partial or stale snapshots", () => {
    expect(
      isPublicCorporation({ ...profile, member_count: 0, stale: true }),
    ).toBe(true);
    expect(
      isPublicCorporation({
        ...profile,
        alliance: { id: 2, name: "A", ticker: "A", corporation_count: null },
      }),
    ).toBe(true);
  });
});
