import { describe, expect, it } from "vitest";
import type { Row } from "./api";
import { grossAmount, journalRows } from "./journal-rows";

const source: Row = {
  id: "53",
  entry_key: "a",
  division: 0,
  observed_at: "2026-10-17T00:00:00Z",
  amount: "3025360.79",
  tax: "432194.39875",
  balance: "10000000.01",
  tax_receiver_id: "10",
  first_party_id: "20",
  second_party_id: "123",
};

describe("separate income and corporation tax rows", () => {
  it("shows gross income and tax as separate rows with the original net and final balance intact", () => {
    const rows = journalRows([source], "123");
    expect(rows.map((r) => [r.display_kind, r.amount, r.balance])).toEqual([
      ["gross", "3457555.18875", undefined],
      ["tax", "-432194.39875", "10000000.01"],
    ]);
    expect(grossAmount(rows[0].amount, rows[1].amount)).toBe("3025360.79000");
    expect(rows[1].first_party_id).toBe("123");
    expect(rows[1].second_party_id).toBe("10");
    expect(
      rows.every(
        (r) =>
          r.source === source &&
          r.id === source.id &&
          r.entry_key === source.entry_key,
      ),
    ).toBe(true);
    expect(source.amount).toBe("3025360.79");
  });
  it("keeps only the matching display rows in income, expense and tax filters", () => {
    const debit = { ...source, id: "52", amount: "-7", tax: undefined };
    expect(
      journalRows([source, debit], "123", "out").map((r) => r.amount),
    ).toEqual(["-432194.39875", "-7"]);
    expect(
      journalRows([source, debit], "123", "tax").map((r) => r.display_kind),
    ).toEqual(["tax"]);
    expect(journalRows([source], "123", "in").map((r) => r.amount)).toEqual([
      "3457555.18875",
    ]);
  });
  it("does not split corporation observations, absent taxes or tax refunds", () => {
    expect(journalRows([source])).toEqual([source]);
    for (const tax of [undefined, "0", "-1"]) {
      const row = { ...source, tax };
      expect(journalRows([row], "123")).toEqual([row]);
    }
  });
  it("preserves large amounts, scientific notation and full withholding without binary floats", () => {
    expect(grossAmount("9007199254740993.01", "0.00001")).toBe(
      "9007199254740993.01001",
    );
    expect(grossAmount("1e-8", "2e-9")).toBe("0.000000012");
    expect(grossAmount("1e3", "0.1")).toBe("1000.1");
    expect(
      journalRows([{ ...source, amount: "0", tax: "5" }], "123", "in")[0]
        .amount,
    ).toBe("5");
    expect(grossAmount(null, "5")).toBeUndefined();
  });
  it("keeps paired variants on their source page and distinguishes their row keys", () => {
    const first = journalRows(
      Array.from({ length: 50 }, (_, i) => ({
        ...source,
        entry_key: String(i),
      })),
      "123",
    );
    expect(first).toHaveLength(100);
    expect(
      new Set(first.map((r) => `${r.id}:${r.entry_key}:${r.display_kind}`))
        .size,
    ).toBe(100);
    expect(journalRows([{ ...source, entry_key: "next" }], "123")).toHaveLength(
      2,
    );
  });
});
