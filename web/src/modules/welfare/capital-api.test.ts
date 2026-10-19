import { expect, it } from "vitest";
import { capitalAmount, isCapitalCheck } from "./capital-api";
import type { Case } from "./api";

it("requires verified purchase data for a ready preview", () => {
  const value = { state: "ready", policy_version: "1", rate_bps: 1250 };
  expect(isCapitalCheck(value)).toBe(false);
  const purchase = {
    contract_id: "901",
    character_id: "123",
    character_name: "Pilot",
    ship_type_id: "23919",
    ship_name: "Aeon",
    price_minor: 1000000000000,
    award_minor: 125000000000,
    completed_at: "2026-10-19T00:00:00Z",
  };
  expect(isCapitalCheck({ ...value, purchase })).toBe(true);
  for (const award_minor of [-1, 1.5, 99, "100", Number.MAX_SAFE_INTEGER + 1])
    expect(
      isCapitalCheck({ ...value, purchase: { ...purchase, award_minor } }),
    ).toBe(false);
  expect(isCapitalCheck({ ...value, state: "future_unknown", purchase })).toBe(
    false,
  );
});

it("uses frozen integer rates and preserves approved awards", () => {
  const item = {
    kind: "titan",
    award_minor: 0,
    detail: {
      base_minor: 99999999999999,
      rule: { subsidy_rate_bps: 3333 },
      purchase: {},
    },
  } as Case;
  expect(capitalAmount(item)).toBe(33329999999999);
  expect(capitalAmount({ ...item, award_minor: 12345 })).toBe(12345);
  expect(
    capitalAmount({ ...item, detail: { ...item.detail, purchase: undefined } }),
  ).toBe(0);
});
