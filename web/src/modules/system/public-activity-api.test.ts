import { describe, expect, it } from "vitest";
import { isPublicActivity } from "./public-activity-api";

describe("public activity projection", () => {
  const base = { corporation_id: 98530802, combat: null, online: null };
  it("supports independently unavailable sources and rejects the wrong target", () => {
    expect(isPublicActivity(base)).toBe(true);
    expect(isPublicActivity({ ...base, corporation_id: 1 })).toBe(false);
    expect(isPublicActivity({ corporation_id: 98530802 })).toBe(false);
  });
  it("rejects fabricated online zero without coverage and inconsistent counts", () => {
    const online = { characters: 2, covered_characters: 3, bound_characters: 5, updated_at: "2026-09-23T00:00:00Z", expires_at: "2026-09-23T00:01:00Z" };
    expect(isPublicActivity({ ...base, online })).toBe(true);
    expect(isPublicActivity({ ...base, online: { ...online, characters: 0, covered_characters: 0 } })).toBe(false);
    expect(isPublicActivity({ ...base, online: { ...online, characters: null, covered_characters: 0 } })).toBe(true);
    expect(isPublicActivity({ ...base, online: { ...online, characters: 4 } })).toBe(false);
    expect(isPublicActivity({ ...base, online: { ...online, bound_characters: 1 } })).toBe(false);
  });
});
