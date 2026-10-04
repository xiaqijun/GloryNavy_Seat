import { describe, expect, it } from "vitest";
import { mergeUsageRecords } from "./usage-merge";

const base = {
  id: "1",
  grant_id: "g",
  interval_id: "i",
  ended_at: "2026-10-04T00:00:10Z",
  duration_seconds: 10,
  coins_minor: 100,
  state: "settled" as const,
  unit_seconds: 3600,
  unit_price_minor: 360000,
  expires_at: "2026-10-05T00:00:00Z",
  system_id: "legacy:s-kswl",
};

describe("sentry usage display projection", () => {
  it("combines a charge and monitoring reward for one continuous system interval", () => {
    const records = mergeUsageRecords(
      [{ ...base, started_at: "2026-10-04T00:00:00Z" }],
      [{
        contribution_id: "r",
        started_at: "2026-10-04T00:00:10Z",
        ended_at: "2026-10-04T00:00:20Z",
        duration_seconds: 10,
        coins_minor: 50,
        system_id: "S-KSWL",
        system_name: "S-KSWL",
      }],
    );
    expect(records).toHaveLength(1);
    expect(records[0]).toMatchObject({ duration_seconds: 20, charge_minor: 100, reward_minor: 50, system_name: "S-KSWL" });
  });

  it("keeps separated time gaps as separate records", () => {
    const records = mergeUsageRecords(
      [{ ...base, started_at: "2026-10-04T00:00:00Z" }],
      [{
        contribution_id: "r",
        started_at: "2026-10-04T00:01:00Z",
        ended_at: "2026-10-04T00:01:10Z",
        duration_seconds: 10,
        coins_minor: 50,
        system_id: "S-KSWL",
        system_name: "S-KSWL",
      }],
    );
    expect(records).toHaveLength(2);
  });
});
