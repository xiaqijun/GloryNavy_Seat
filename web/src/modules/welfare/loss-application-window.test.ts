import { describe, expect, it } from "vitest";
import { applicationDays, lossApplicationExpired } from "./loss-application-window";

describe("rolling loss application window", () => {
  it("uses whole 24-hour days and includes the exact boundary", () => {
    const at = "2026-01-30T00:00:00Z";
    const boundary = Date.parse("2026-03-01T00:00:00Z");
    expect(lossApplicationExpired(at, undefined, boundary)).toBe(false);
    expect(lossApplicationExpired(at, 30, boundary + 1)).toBe(true);
    expect(lossApplicationExpired("2026-01-30T08:00:00+08:00", 30, boundary)).toBe(false);
    expect(lossApplicationExpired(at, 14, Date.parse(at) + 14 * 86_400_000)).toBe(false);
    expect(lossApplicationExpired(at, 14, Date.parse(at) + 14 * 86_400_000 + 1)).toBe(true);
  });
  it("requires a positive whole number of days", () => {
    for (const value of ["", "0", "-1", "1.5", "3651", "NaN"]) expect(() => applicationDays(value)).toThrow();
    for (const value of ["1", "14", "30", "3650"]) expect(applicationDays(value)).toBe(Number(value));
  });
});
