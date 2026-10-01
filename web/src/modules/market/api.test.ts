import { expect, it } from "vitest";
import { isAppraisal } from "./api";
it("rejects numeric/invalid currency and unknown rows", () => {
  const base = {
    lines: [],
    totals: { buy: "0.00", mid: "0.00", sell: "0.00" },
    adjusted: { buy: "0.00", mid: "0.00", sell: "0.00" },
    ratio_bps: 10000,
    complete: true,
  };
  expect(isAppraisal(base)).toBe(true);
  expect(isAppraisal({ ...base, totals: { ...base.totals, mid: 0 } })).toBe(
    false,
  );
  expect(isAppraisal({ ...base, totals: { ...base.totals, mid: "NaN" } })).toBe(
    false,
  );
  expect(isAppraisal({ ...base, lines: [{}] })).toBe(false);
});
