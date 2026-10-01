import { describe, expect, it } from "vitest";
import { isCoinQuote } from "./coin-api";
describe("coin quote boundary", () => {
  const quote = {
    token: "a".repeat(64),
    mode: "manual",
    points: 6,
    converted: 2,
    pending: 4,
    coins_minor: 1000,
    characters: 2,
  };
  it("accepts empty and pending quotes", () => {
    expect(isCoinQuote(quote)).toBe(true);
    expect(
      isCoinQuote({ ...quote, pending: 0, coins_minor: 0, characters: 0 }),
    ).toBe(true);
  });
  it("rejects malformed token and unsafe amounts", () => {
    for (const data of [
      { ...quote, token: "" },
      { ...quote, coins_minor: Number.MAX_SAFE_INTEGER + 1 },
      { ...quote, pending: -1 },
      { ...quote, mode: "unknown" },
      { ...quote, points: 1.5 },
    ])
      expect(isCoinQuote(data)).toBe(false);
  });
});
