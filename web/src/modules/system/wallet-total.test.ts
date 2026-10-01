import { describe, expect, it } from "vitest";
import { sumWalletBalances } from "./wallet-total";
describe("wallet totals retain decimal precision", () => {
  it("adds scientific notation, fractions and amounts beyond Number precision", () => {
    expect(sumWalletBalances(["9007199254740993.12", "0.01", "1e-2"])).toBe(
      "9007199254740993.14",
    );
    expect(sumWalletBalances(["-0.03", "0.02"])).toBe("-0.01");
    expect(sumWalletBalances(["1.001", "-1"])).toBe("0.001");
  });
  it("distinguishes missing values from actual zero", () => {
    expect(sumWalletBalances([])).toBeNull();
    expect(sumWalletBalances(["1", null])).toBeNull();
    expect(sumWalletBalances(["bad"])).toBeNull();
    expect(sumWalletBalances(["0", "0.00"])).toBe("0.00");
  });
});
