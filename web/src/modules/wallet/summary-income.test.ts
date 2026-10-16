import { expect, it } from "vitest";
import { isSummary } from "./api";

it("validates exact transfer-adjusted totals while retaining old summary compatibility", () => {
  const item = {
    owner_id: "123",
    balance: "0.00",
    observed_at: null,
    income: "9999999999999999.12",
    expense: "1000.01",
  };
  expect(isSummary({ items: [item] })).toBe(true);
  expect(
    isSummary({
      items: [
        { ...item, earned_income: "999999999999999.12", spent_expense: "0.01" },
      ],
    }),
  ).toBe(true);
  expect(isSummary({ items: [{ ...item, earned_income: 100 }] })).toBe(false);
  expect(isSummary({ items: [{ ...item, spent_expense: null }] })).toBe(false);
});
