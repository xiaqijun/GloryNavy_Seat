import { describe, expect, it } from "vitest";
import { battleReportURL, isBattleReport } from "./battle-report";

describe("battle reports", () => {
  it("uses only an exact positive KM ID on the fixed report host", () => {
    expect(battleReportURL("139267042")).toBe("https://br.scers.cn/q?mode=expand&km=139267042");
    for (const id of ["0", "-1", "01", "1&redirect=evil", "9223372036854775808", "https://example.com", "1.5"]) expect(battleReportURL(id)).toBe("");
  });
  it("rejects incomplete or invalid stats instead of presenting fabricated zeroes", () => {
    expect(isBattleReport({ status: "pending" })).toBe(true);
    expect(isBattleReport({ status: "unavailable" })).toBe(true);
    expect(isBattleReport({ status: "ready" })).toBe(false);
    const value = { status: "ready", summary: { start: "2026-10-19T00:00:00Z", end: "2026-10-19T01:00:00Z", observed_at: "2026-10-19T02:00:00Z", systems: ["15W-GC"], kill_count: 80, total_isk: 8e9, teams: [{ id: "A", name: "Alliance A", pilots: 10, kills: 30, losses: 50, kills_isk: 3e9, losses_isk: 5e9, own: true }] } };
    expect(isBattleReport(value)).toBe(true);
    value.summary.teams[0].losses_isk = NaN;
    expect(isBattleReport(value)).toBe(false);
  });
});
