import { describe, expect, it } from "vitest";
import { hasWithheldTax } from "./tax";

describe("withheld tax decimal evidence", () => {
  it("preserves positive values including small fractions and large ISK amounts", () => {
    for (const tax of [
      "432194.39875",
      "0.00000001",
      "1e-20",
      "9007199254740993.01",
      "000.10",
    ])
      expect(hasWithheldTax(tax)).toBe(true);
  });
  it("does not invent tax for zero, refunds, absent or malformed values", () => {
    for (const tax of [
      undefined,
      null,
      "",
      "0",
      "0.000",
      "0e9",
      "-0.1",
      "NaN",
      "1e999",
      "1 ISK",
    ])
      expect(hasWithheldTax(tax)).toBe(false);
  });
});
