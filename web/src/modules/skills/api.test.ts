import { describe, it, expect } from "vitest";
import { isSnapshot, isPlan } from "./api";
describe("skills API validation", () => {
  it("preserves unknown values and rejects invalid levels", () => {
    const meta = {
      status: "pending",
      reason: "",
      observed_at: null,
      valid_until: null,
    };
    const v = {
      skills_meta: meta,
      queue_meta: meta,
      skills: [],
      queue: [],
      total_sp: null,
      unallocated_sp: null,
      calculated_at: new Date().toISOString(),
    };
    expect(isSnapshot(v)).toBe(true);
    expect(isSnapshot({ ...v, total_sp: -1 })).toBe(false);
    expect(
      isSnapshot({
        ...v,
        skills: [
          {
            id: "3300",
            name: "射击学",
            group: "炮术",
            trained: 6,
            active: 5,
            points: 1,
            queue_applied: false,
          },
        ],
      }),
    ).toBe(false);
  });
  it("rejects numeric IDs and out-of-range requirements", () => {
    const v = {
      id: "1",
      corporation_id: "900",
      version: "1",
      name: "基础",
      requirements: [{ skill_id: "3300", level: 5 }],
      updated_at: "2026-09-15T00:00:00Z",
    };
    expect(isPlan(v)).toBe(true);
    expect(isPlan({ ...v, id: 1 })).toBe(false);
    expect(
      isPlan({ ...v, requirements: [{ skill_id: "3300", level: 0 }] }),
    ).toBe(false);
  });
});
