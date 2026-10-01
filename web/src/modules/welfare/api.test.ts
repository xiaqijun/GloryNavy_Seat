import { describe, it, expect } from "vitest";
import {
  minor,
  isQuote,
  isDeliveryCandidates,
  isValuation,
  isGrowthRewards,
} from "./api";
it("validates configurable growth rewards without rounding coins", () => {
  const rewards = {
    fittings: [{ fitting_id: "1", quantity: 2 }],
    items: [{ type_id: "34", quantity: 100 }],
    coins_minor: 125,
  };
  expect(isGrowthRewards(rewards)).toBe(true);
  for (const change of [
    { coins_minor: 1.25 },
    { coins_minor: -1 },
    { coins_minor: 1e12 + 1 },
    { fittings: [{ fitting_id: "1", quantity: 0 }] },
    { items: [{ type_id: "0", quantity: 1 }] },
  ])
    expect(isGrowthRewards({ ...rewards, ...change })).toBe(false);
});
describe("welfare amounts", () => {
  it("rejects malformed valuation snapshots", () => {
    const v = {
      source: "market",
      state: "incomplete",
      reason: "缺价",
      at: "2026-09-19T00:00:00Z",
      amount_minor: 0,
      settings_version: "1",
    };
    expect(isValuation(v)).toBe(true);
    expect(isValuation({ ...v, amount_minor: "100" })).toBe(false);
    expect(isValuation({ ...v, market: { complete: true } })).toBe(false);
    expect(isValuation({ ...v, at: "invalid" })).toBe(false);
  });
  it("rejects partial or untyped delivery evidence", () => {
    expect(isDeliveryCandidates({ items: [] })).toBe(true);
    expect(
      isDeliveryCandidates({
        items: [
          {
            can_link: true,
            reason: "",
            contract: { id: "1", status: "finished" },
          },
        ],
      }),
    ).toBe(false);
    expect(
      isDeliveryCandidates({
        items: [{ can_link: "true", reason: "", contract: {} }],
      }),
    ).toBe(false);
  });
  it("converts decimal text without float rounding", () => {
    expect(minor("12.50")).toBe(1250);
    expect(minor("0.01")).toBe(1);
    expect(minor("500000000")).toBe(50000000000);
  });
  it("rejects ambiguous precision and unsafe amounts", () => {
    for (const v of ["1.001", "-1", "1e3", "Infinity", "9007199254740992", ""])
      expect(() => minor(v)).toThrow();
  });
  it("requires complete grant quote", () => {
    expect(
      isQuote({
        token: "x",
        total_minor: 100,
        lines: [{ account_id: "a", amount_minor: 100 }],
      }),
    ).toBe(true);
    expect(isQuote({ token: "x", total_minor: 1.1, lines: [] })).toBe(false);
  });
});
